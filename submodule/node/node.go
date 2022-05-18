package node

import (
	"context"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/gorilla/mux"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"

	"github.com/memoio/go-mefs-v2/api"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/repo"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/service/netapp"
	mauth "github.com/memoio/go-mefs-v2/submodule/auth"
	mconfig "github.com/memoio/go-mefs-v2/submodule/config"
	"github.com/memoio/go-mefs-v2/submodule/connect/settle"
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

	*txPool.PushPool

	*settle.ContractMgr

	tx.Store

	repo.Repo

	ctx context.Context

	HttpHandle *mux.Router

	roleID  uint64
	groupID uint64
	pw      string
	version string

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
	if n.Repo.Config().Net.Name == "test" {
		go n.OpenTest()
	} else {
		n.RoleMgr.Start()
	}

	n.TxMsgHandle.Register(n.TxMsgHandler)
	n.BlockHandle.Register(n.TxBlockHandler)

	n.GenericService.Register(pb.NetMessage_SayHello, n.DefaultHandler)
	n.MsgHandle.Register(pb.NetMessage_Get, n.HandleGet)

	n.HttpHandle.Handle("/debug/metrics", metrics.Exporter())
	n.HttpHandle.PathPrefix("/").Handler(http.DefaultServeMux)

	if n.Perm {
		n.RPCServer.Register("Memoriae", api.PermissionedFullAPI(metrics.MetricedFullAPI(n)))
	} else {
		n.RPCServer.Register("Memoriae", metrics.MetricedFullAPI(n))
	}

	go func() {
		// wait for sync
		n.PushPool.Start()
		for {
			if n.PushPool.Ready() {
				break
			} else {
				logger.Debug("wait for sync")
				time.Sleep(5 * time.Second)
			}
		}
	}()

	logger.Info("start base node: ", n.roleID)

	return nil
}

func (n *BaseNode) Ready(ctx context.Context) bool {
	return true
}

func (n *BaseNode) Stop(ctx context.Context) error {
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
