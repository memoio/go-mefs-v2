package node

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/libp2p/go-libp2p-core/host"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"

	"github.com/memoio/go-mefs-v2/app/api"
	"github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/repo"
	"github.com/memoio/go-mefs-v2/service/netapp"
	mauth "github.com/memoio/go-mefs-v2/submodule/auth"
	mconfig "github.com/memoio/go-mefs-v2/submodule/config"
	"github.com/memoio/go-mefs-v2/submodule/network"
	"github.com/memoio/go-mefs-v2/submodule/wallet"
)

var logger = log.Logger("basenode")

var _ api.FullNode = (*BaseNode)(nil)

type BaseNode struct {
	*network.NetworkSubmodule

	*wallet.LocalWallet

	*mauth.JwtAuth

	*mconfig.ConfigModule

	*netapp.NetServiceImpl

	*jsonrpc.RPCServer

	repo.Repo

	ctx context.Context

	ShutdownChan chan struct{}

	IsOnline bool
}

// Start boots up the node.
func (n *BaseNode) Start() error {
	n.RPCServer = jsonrpc.NewServer()
	n.RPCServer.Register("Memoriae", api.PermissionedFullAPI(n))

	return nil
}

func (n *BaseNode) Stop(ctx context.Context) {
	n.GenericService.Subscriber.Close()

	n.NetworkSubmodule.Stop(ctx)

	if err := n.Repo.Close(); err != nil {
		fmt.Printf("error closing repo: %s\n", err)
	}

	fmt.Println("\nstopping Memoriae :(")
}

func (n *BaseNode) RunDaemon(ready chan interface{}) error {
	cfg := n.Repo.Config()
	apiAddr, err := ma.NewMultiaddr(cfg.API.APIAddress)
	if err != nil {
		return err
	}

	// Listen on the configured address in order to bind the port number in case it has
	// been configured as zero (i.e. OS-provided)
	apiListener, err := manet.Listen(apiAddr) //nolint
	if err != nil {
		return err
	}

	netListener := manet.NetListener(apiListener) //nolint

	handler := http.NewServeMux()
	handler.Handle("/rpc/v0", n.RPCServer)

	// todo: add auth
	ah := &auth.Handler{
		Verify: n.AuthVerify,
		Next:   handler.ServeHTTP,
	}

	apiserv := &http.Server{
		Handler: ah,
	}

	cfg.API.APIAddress = apiListener.Multiaddr().String()
	if err := n.Repo.SetAPIAddr(cfg.API.APIAddress); err != nil {
		return err
	}

	var terminate = make(chan os.Signal, 1)
	signal.Notify(terminate, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(terminate)

	n.ShutdownChan = make(chan struct{})

	close(ready)

	go func() {
		select {
		case <-n.ShutdownChan:
			logger.Warn("received shutdown")
		case <-terminate:
			logger.Warn("received shutdown signal")
		}

		logger.Warn("shutdown...")
		err = apiserv.Shutdown(n.ctx)
		if err != nil {
			return
		}
		n.Stop(n.ctx)
	}()
	return apiserv.Serve(netListener)
}

func (n *BaseNode) Online() bool {
	return n.IsOnline
}

func (n *BaseNode) GetHost() host.Host {
	return n.Host
}
