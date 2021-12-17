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

	*txPool.PushPool

	repo.Repo

	ctx context.Context

	httpHandle *mux.Router

	roleID  uint64
	groupID uint64

	isOnline bool
}

func (n *BaseNode) RoleID() uint64 {
	return n.roleID
}

func (n *BaseNode) GroupID() uint64 {
	return n.groupID
}

// Start boots up the node.
func (n *BaseNode) Start() error {

	go n.OpenTest()

	n.TxMsgHandle.Register(n.TxMsgHandler)
	n.BlockHandle.Register(n.TxBlockHandler)

	n.MsgHandle.Register(pb.NetMessage_Get, n.HandleGet)

	n.RPCServer.Register("Memoriae", api.PermissionedFullAPI(metrics.MetricedFullAPI(n)))

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

	logger.Info("start base node")

	return nil
}

func (n *BaseNode) Stop(ctx context.Context) error {
	n.GenericService.MsgHandle.Close()

	n.NetworkSubmodule.Stop(ctx)

	// stop other module

	err := n.Repo.Close()
	if err != nil {
		logger.Errorf("error closing repo: %s", err)
	}

	logger.Info("stopping Memoriae :(")
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

	// add auth
	ah := &auth.Handler{
		Verify: n.AuthVerify,
		Next:   n.httpHandle.ServeHTTP,
	}

	apiserv := &http.Server{
		Handler: ah,
	}

	cfg.API.Address = apiListener.Multiaddr().String()
	if err := n.Repo.SetAPIAddr(cfg.API.Address); err != nil {
		return err
	}

	var terminate = make(chan os.Signal, 2)
	shutdownChan := make(chan struct{})
	go func() {
		select {
		case <-shutdownChan:
			logger.Warn("received shutdown")
		case sig := <-terminate:
			logger.Warn("received shutdown signal: ", sig)
		}

		logger.Warn("shutdown...")
		err = apiserv.Shutdown(context.TODO())
		if err != nil {
			logger.Errorf("shut down api server failed: %s", err)
		}
		err = n.Stop(context.TODO())
		if err != nil {
			logger.Errorf("shut down node failed: %s", err)
		}

		logger.Info("shutdown successful")
		close(shutdownChan)
	}()
	signal.Notify(terminate, syscall.SIGTERM, syscall.SIGINT)

	err = apiserv.Serve(netListener)
	if err == http.ErrServerClosed {
		<-shutdownChan
		return nil
	}

	return err
}

func (n *BaseNode) Online() bool {
	return n.isOnline
}
