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
	"github.com/pkg/errors"

	"github.com/memoio/go-mefs-v2/app/api"
	"github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/repo"
	"github.com/memoio/go-mefs-v2/lib/rpc_builder"
	"github.com/memoio/go-mefs-v2/service"
	mauth "github.com/memoio/go-mefs-v2/submodule/auth"
	mconfig "github.com/memoio/go-mefs-v2/submodule/config"
	"github.com/memoio/go-mefs-v2/submodule/network"
	"github.com/memoio/go-mefs-v2/submodule/wallet"
)

var logger = log.Logger("basenode")

type BaseNode struct {
	*network.NetworkSubmodule

	*wallet.LocalWallet

	*mauth.JwtAuth

	*mconfig.ConfigModule

	ctx context.Context

	repo repo.Repo

	Service service.CoreService

	jsonRPCService *jsonrpc.RPCServer

	IsOnline bool
}

// Start boots up the node.
func (n *BaseNode) Start(ctx context.Context) error {
	apiBuilder := rpc_builder.NewBuiler()
	apiBuilder.NameSpace("Memoriae")
	err := apiBuilder.AddServices(
		n.NetworkSubmodule,
		n.LocalWallet,
		n.JwtAuth,
		n.ConfigModule,
	)
	if err != nil {
		return errors.Wrap(err, "add service failed ")
	}
	n.jsonRPCService = apiBuilder.Build()
	return nil
}

func (n *BaseNode) Stop(ctx context.Context) {
	n.NetworkSubmodule.Stop(ctx)

	if err := n.repo.Close(); err != nil {
		fmt.Printf("error closing repo: %s\n", err)
	}

	fmt.Println("\nstopping Memoriae :(")
}

func (n *BaseNode) RunRPCAndWait(ctx context.Context, ready chan interface{}) error {
	cfg := n.repo.Config()
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

	rpcServer := jsonrpc.NewServer()
	rpcServer.Register("Memoriae", api.PermissionedFullAPI(n))

	handler.Handle("/rpc/v0", rpcServer)

	// todo: add auth
	ah := &auth.Handler{
		Verify: n.AuthVerify,
		Next:   handler.ServeHTTP,
	}

	apiserv := &http.Server{
		Handler: ah,
	}

	cfg.API.APIAddress = apiListener.Multiaddr().String()
	if err := n.repo.SetAPIAddr(cfg.API.APIAddress); err != nil {
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
		err = apiserv.Shutdown(ctx)
		if err != nil {
			return
		}
		n.Stop(ctx)
	}()
	return apiserv.Serve(netListener)
}

func (n *BaseNode) Online() bool {
	return n.IsOnline
}

func (n *BaseNode) GetHost() host.Host {
	return n.Host
}
