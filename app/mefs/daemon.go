package main

import (
	"fmt"
	_ "net/http/pprof"
	"os"
	"runtime"
	"sort"
	"strings"

	mprome "github.com/ipfs/go-metrics-prometheus"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/urfave/cli/v2"

	"github.com/memoio/go-mefs-v2/app/cmd"
	"github.com/memoio/go-mefs-v2/app/minit"
	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/repo"
	basenode "github.com/memoio/go-mefs-v2/submodule/node"
)

const (
	bootstrapOptionKwd = "bootstrap"
	apiAddrKwd         = "api"
	swarmPortKwd       = "swarm-port"
)

type Node interface {
	Start() error
	RunDaemon(chan interface{}) error
	Online() bool
	GetHost() host.Host
}

var DaemonCmd = &cli.Command{
	Name:  "daemon",
	Usage: "Run a network-connected Memoriae keeper.",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "password",
			Usage: "password for asset private key",
			Value: "memoriae",
		},
		&cli.StringFlag{
			Name:  apiAddrKwd,
			Usage: "set the api port to use",
			Value: "7000",
		},
		&cli.StringFlag{
			Name:  swarmPortKwd,
			Usage: "set the swarm port to use",
			Value: "12000",
		},
	},
	Action: func(cctx *cli.Context) error {
		return daemonFunc(cctx)
	},
}

func daemonFunc(cctx *cli.Context) (_err error) {
	err := mprome.Inject()
	if err != nil {
		fmt.Errorf("Injecting prometheus handler for metrics failed with message: %s\n", err.Error())
		return err
	}

	// let the user know we're going.
	fmt.Printf("Initializing daemon...\n")

	defer func() {
		if _err != nil {
			// Print an extra line before any errors. This could go
			// in the commands lib but doesn't really make sense for
			// all commands.
			fmt.Println()
		}
	}()

	printVersion()

	stopFunc, err := minit.ProfileIfEnabled()
	if err != nil {
		return err
	}
	defer stopFunc()

	repoDir := cctx.String(cmd.FlagNodeRepo)

	// third precedence is config file.
	rep, err := repo.NewFSRepo(repoDir, nil)
	if err != nil {
		return err
	}

	// The node will also close the repo but there are many places we could
	// fail before we get to that. It can't hurt to close it twice.
	defer rep.Close()

	config := rep.Config()

	// second highest precedence is env vars.
	if envAPI := os.Getenv("MEMO_API"); envAPI != "" {
		config.API.APIAddress = envAPI
	}

	if swarmPort := cctx.String(swarmPortKwd); swarmPort != "" {
		changed := make([]string, 0, len(config.Net.Addresses))
		for _, swarmAddr := range config.Net.Addresses {
			strs := strings.Split(swarmAddr, "/")
			for i, str := range strs {
				if str == "tcp" || str == "udp" {
					strs[i+1] = swarmPort
				}
			}
			changed = append(changed, strings.Join(strs, "/"))
		}
		config.Net.Addresses = changed
	}

	rep.ReplaceConfig(config)

	var node Node
	switch config.Identity.Role {
	default:
		opts, err := basenode.OptionsFromRepo(rep)
		if err != nil {
			return err
		}

		password := cctx.String("password")
		opts = append(opts, basenode.SetPassword(password))

		node, err = basenode.New(cctx.Context, opts...)
		if err != nil {
			return err
		}
	}

	printSwarmAddrs(node)

	// Start the node
	if err := node.Start(); err != nil {
		return err
	}

	// Run API server around the keeper.
	ready := make(chan interface{}, 1)
	go func() {
		<-ready

		// The daemon is *finally* ready.
		fmt.Printf("Network PeerID is %s\n", node.GetHost().ID().String())
		fmt.Printf("Daemon is ready\n")
	}()

	return node.RunDaemon(ready)
}

func printVersion() {
	v := build.UserVersion()
	fmt.Printf("Memoriae version: %s\n", v)
	fmt.Printf("System version: %s\n", runtime.GOARCH+"/"+runtime.GOOS)
	fmt.Printf("Golang version: %s\n", runtime.Version())
}

// printSwarmAddrs prints the addresses of the host
func printSwarmAddrs(node Node) {
	if !node.Online() {
		fmt.Println("Swarm not listening, running in offline mode.")
		return
	}

	var lisAddrs []string
	ifaceAddrs, err := node.GetHost().Network().InterfaceListenAddresses()
	if err != nil {
		fmt.Errorf("failed to read listening addresses: %s", err)
	}
	for _, addr := range ifaceAddrs {
		lisAddrs = append(lisAddrs, addr.String())
	}
	sort.Strings(lisAddrs)
	for _, addr := range lisAddrs {
		fmt.Printf("Swarm listening on %s\n", addr)
	}

	var addrs []string
	for _, addr := range node.GetHost().Addrs() {
		addrs = append(addrs, addr.String())
	}
	sort.Strings(addrs)
	for _, addr := range addrs {
		fmt.Printf("Swarm announcing %s\n", addr)
	}
}
