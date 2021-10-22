package node

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/libp2p/go-libp2p-core/host"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/pkg/errors"

	"github.com/memoio/go-mefs-v2/lib/api_builder"
	"github.com/memoio/go-mefs-v2/lib/repo"
	"github.com/memoio/go-mefs-v2/lib/types/wallet"
	"github.com/memoio/go-mefs-v2/service"
	"github.com/memoio/go-mefs-v2/submodule/network"
)

type BaseNode struct {
	*network.NetworkSubmodule
	wallet.Wallet

	ctx context.Context

	repo repo.Repo

	Service service.CoreService

	jsonRPCService *jsonrpc.RPCServer

	IsOnline bool
}

// Start boots up the node.
func (n *BaseNode) Start(ctx context.Context) error {
	apiBuilder := api_builder.NewBuiler()
	apiBuilder.NameSpace("Memoriae")
	err := apiBuilder.AddServices(
		n.Wallet,
		n.NetworkSubmodule,
	)
	if err != nil {
		return errors.Wrap(err, "add service failed ")
	}
	n.jsonRPCService = apiBuilder.Build()
	return nil
}

func (n *BaseNode) Stop(ctx context.Context) {
	if err := n.repo.Close(); err != nil {
		fmt.Printf("error closing repo: %s\n", err)
	}

	n.NetworkSubmodule.Stop(ctx)

	fmt.Println("\nstopping Memoriae :(")
}

func (n *BaseNode) RunRPCAndWait(ctx context.Context, ready chan interface{}) error {
	var terminate = make(chan os.Signal, 1)
	signal.Notify(terminate, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(terminate)
	// Signal that the sever has started and then wait for a signal to stop.
	apiConfig := n.repo.Config()
	mAddr, err := ma.NewMultiaddr(apiConfig.API.APIAddress)
	if err != nil {
		return err
	}

	// Listen on the configured address in order to bind the port number in case it has
	// been configured as zero (i.e. OS-provided)
	apiListener, err := manet.Listen(mAddr) //nolint
	if err != nil {
		return err
	}

	netListener := manet.NetListener(apiListener) //nolint
	handler := http.NewServeMux()

	err = n.runJsonrpcAPI(ctx, handler)
	if err != nil {
		return err
	}

	apiserv := &http.Server{
		Handler: handler,
	}

	go func() {
		err := apiserv.Serve(netListener) //nolint
		if err != nil && err != http.ErrServerClosed {
			return
		}
	}()

	// Write the resolved API address to the repo
	apiConfig.API.APIAddress = apiListener.Multiaddr().String()
	if err := n.repo.SetAPIAddr(apiConfig.API.APIAddress); err != nil {
		fmt.Errorf("Could not save API address to repo: %s", err)
		return err
	}

	n.ShutdownChan = make(chan struct{})

	close(ready)
	select {
	case <-n.ShutdownChan:
		err = apiserv.Shutdown(ctx)
		if err != nil {
			return err
		}
		n.Stop(ctx)
	case <-terminate:
		err = apiserv.Shutdown(ctx)
		if err != nil {
			return err
		}
		n.Stop(ctx)
	}
	return nil
}

func (n *BaseNode) runJsonrpcAPI(ctx context.Context, handler *http.ServeMux) error { //nolint
	handler.Handle("/rpc/v0", n.jsonRPCService)
	return nil
}

func (n *BaseNode) Online() bool {
	return n.IsOnline
}

func (n *BaseNode) GetHost() host.Host {
	return n.Host
}
