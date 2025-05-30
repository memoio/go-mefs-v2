package node

import (
	"context"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/gorilla/mux"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/api"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/repo"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/service/netapp"
	mauth "github.com/memoio/go-mefs-v2/submodule/auth"
	mconfig "github.com/memoio/go-mefs-v2/submodule/config"
	"github.com/memoio/go-mefs-v2/submodule/metrics"
	"github.com/memoio/go-mefs-v2/submodule/network"
	"github.com/memoio/go-mefs-v2/submodule/role"
	"github.com/memoio/go-mefs-v2/submodule/txPool"
	"github.com/memoio/go-mefs-v2/submodule/wallet"
)

var logger = logging.Logger("basenode")

var _ api.FullNode = (*BaseNode)(nil)

type BaseNode struct {
	*network.NetworkSubmodule

	*wallet.LocalWallet

	*mauth.JwtAuth

	*mconfig.ConfigModule

	*netapp.NetServiceImpl

	*role.RoleMgr

	*jsonrpc.RPCServer
	jsonrpc.ClientCloser

	api.IChainSync

	api.ISettle

	tx.Store

	repo.Repo

	ctx context.Context

	lk      sync.Mutex
	isProxy bool
	LPP     *txPool.PushPool
	rcp     api.IChainPush

	HttpHandle *mux.Router

	RoleType string
	roleID   uint64
	groupID  uint64
	pw       string
	version  string

	isOnline bool
	Perm     bool

	shutdownChan chan struct{}
}

func (n *BaseNode) RoleID() uint64 {
	return n.roleID
}

func (n *BaseNode) GroupID() uint64 {
	return n.groupID
}

func (n *BaseNode) Version(_ context.Context) (string, error) {
	return n.version, nil
}

// Start boots up the node.
func (n *BaseNode) Start(perm bool) error {
	n.Perm = perm
	n.RoleType = "mefs"

	err := n.StartLocal()
	if err != nil {
		return err
	}

	if n.Perm {
		n.RPCServer.Register("Memoriae", api.PermissionedFullAPI(metrics.MetricedFullAPI(n)))
	} else {
		n.RPCServer.Register("Memoriae", metrics.MetricedFullAPI(n))
	}

	go n.WaitForSync()

	logger.Info("start base node: ", n.roleID)

	return nil
}

func (n *BaseNode) StartLocal() error {
	if n.Repo.Config().Net.Name == "test" {
		go n.OpenTest()
	} else {
		n.RoleMgr.Start()
	}

	err := n.checkSpace()
	if err != nil {
		return err
	}

	go n.CheckSpace()

	if n.isProxy {
		if n.RoleType == "keeper" {
			return xerrors.Errorf("not use sync mode for keeper, clear sync api and token in config")
		}
	} else {
		// handle received tx message
		n.BlockHandle.Register(n.TxBlockHandler)
		// start local push pool
		n.LPP.Start()
	}

	n.GenericService.Register(pb.NetMessage_SayHello, n.DefaultHandler)
	n.MsgHandle.Register(pb.NetMessage_Get, n.HandleGet)

	n.HttpHandle.Handle("/debug/metrics", metrics.NewExporter())
	n.HttpHandle.PathPrefix("/").Handler(http.DefaultServeMux)
	return nil
}

func (n *BaseNode) WaitForSync() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			si, err := n.SyncGetInfo(n.ctx)
			if err != nil {
				continue
			}

			logger.Debug("wait sync; pool state: ", si.SyncedHeight, si.RemoteHeight, si.Status)
			if si.SyncedHeight == si.RemoteHeight && si.Status {
				logger.Info("sync complete; pool state: ", si.SyncedHeight, si.RemoteHeight, si.Status)
				return
			}
		case <-n.ctx.Done():
			return
		}
	}
}

func (n *BaseNode) CheckSpace() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			err := n.checkSpace()
			if err != nil {
				logger.Warn("exit due to: ", err)
				n.Shutdown(context.TODO())
			}
		case <-n.ctx.Done():
			return
		}
	}
}

func (n *BaseNode) checkSpace() error {
	dms, err := n.Repo.LocalStoreGetStat(n.ctx, "kv")
	if err != nil {
		return err
	}
	if dms.Free < 1024*1024*1024 {
		return xerrors.Errorf("caution: meta space is not enough, at least 1GB")
	}

	ds, err := n.Repo.LocalStoreGetStat(n.ctx, "data")
	if err != nil {
		return err
	}
	if ds.Free < 10*1024*1024*1024 {
		return xerrors.Errorf("caution: data space is not enough, at least 10GB")
	}

	return nil
}

func (n *BaseNode) Ready(ctx context.Context) bool {
	return true
}

func (n *BaseNode) Stop(ctx context.Context) error {
	if n.isProxy && n.ClientCloser != nil {
		n.ClientCloser()
	}

	// stop handle msg
	n.NetServiceImpl.Stop()

	// stop network
	n.NetworkSubmodule.Stop(ctx)

	// stop module

	// stop repo
	err := n.Repo.Close()
	if err != nil {
		logger.Errorf("error closing repo: %s", err)
	}

	logger.Info("stopping memoriae node :(")
	return nil
}

func (n *BaseNode) RunDaemon() error {
	cfg := n.Repo.Config()
	apiAddr, err := ma.NewMultiaddr(cfg.API.Address)
	if err != nil {
		return err
	}

	apiListener, err := manet.Listen(apiAddr)
	if err != nil {
		return err
	}

	netListener := manet.NetListener(apiListener) //nolint

	apiserv := &http.Server{
		Handler: n.HttpHandle,
	}
	if n.Perm {
		// add auth
		ah := &auth.Handler{
			Verify: n.AuthVerify,
			Next:   n.HttpHandle.ServeHTTP,
		}

		apiserv = &http.Server{
			Handler: ah,
		}
	}

	cfg.API.Address = apiListener.Multiaddr().String()
	err = n.Repo.SetAPIAddr(cfg.API.Address)
	if err != nil {
		return err
	}

	var terminate = make(chan os.Signal, 2)
	go func() {
		select {
		case <-n.shutdownChan:
			logger.Warn("received shutdown chan")
		case sig := <-terminate:
			logger.Warn("received shutdown signal: ", sig)
		}

		logger.Warn("shutdown ...")
		// stop api service
		ctx, cancle1 := context.WithTimeout(context.TODO(), 1*time.Minute)
		defer cancle1()
		err = apiserv.Shutdown(ctx)
		if err != nil {
			logger.Errorf("shutdown api server failed: %s", err)
		}

		ctx, cancle2 := context.WithTimeout(context.TODO(), 1*time.Minute)
		defer cancle2()
		// stop node
		err = n.Stop(ctx)
		if err != nil {
			logger.Errorf("shutdown node failed: %s", err)
		}

		logger.Info("shutdown successfully")
		close(n.shutdownChan)
	}()
	signal.Notify(terminate, syscall.SIGTERM, syscall.SIGINT)

	err = apiserv.Serve(netListener)
	if err == http.ErrServerClosed {
		<-n.shutdownChan
		return nil
	}

	return err
}

func (n *BaseNode) Online() bool {
	return n.isOnline
}

func (n *BaseNode) PassWord() string {
	return n.pw
}

func (n *BaseNode) Shutdown(ctx context.Context) error {
	n.shutdownChan <- struct{}{}
	return nil
}

func (n *BaseNode) LogSetLevel(ctx context.Context, level string) error {
	return logging.SetLogLevel(level)
}
