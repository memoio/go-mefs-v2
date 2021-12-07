package main

import (
	_ "net/http/pprof"
	"strings"

	mprome "github.com/ipfs/go-metrics-prometheus"
	"github.com/urfave/cli/v2"

	"github.com/memoio/go-mefs-v2/app/cmd"
	"github.com/memoio/go-mefs-v2/app/minit"
	"github.com/memoio/go-mefs-v2/lib/repo"
	"github.com/memoio/go-mefs-v2/service/provider"
	basenode "github.com/memoio/go-mefs-v2/submodule/node"
)

const (
	apiAddrKwd   = "api"
	swarmPortKwd = "swarm-port"
)

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
			Usage: "set the api addr to use",
			Value: "/ip4/127.0.0.1/tcp/8002",
		},
		&cli.StringFlag{
			Name:  swarmPortKwd,
			Usage: "set the swarm port to use",
			Value: "7002",
		},
	},
	Action: func(cctx *cli.Context) error {
		return daemonFunc(cctx)
	},
}

func daemonFunc(cctx *cli.Context) (_err error) {
	err := mprome.Inject()
	if err != nil {
		logger.Errorf("Injecting prometheus handler for metrics failed with message: %s", err)
		return err
	}

	logger.Info("Initializing daemon...")

	minit.PrintVersion()

	stopFunc, err := minit.ProfileIfEnabled()
	if err != nil {
		return err
	}
	defer stopFunc()

	repoDir := cctx.String(cmd.FlagNodeRepo)
	rep, err := repo.NewFSRepo(repoDir, nil)
	if err != nil {
		return err
	}

	defer rep.Close()

	// handle config
	config := rep.Config()

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

	if apiAddr := cctx.String(apiAddrKwd); apiAddr != "" {
		config.API.Address = apiAddr
	}

	rep.ReplaceConfig(config)

	var node minit.Node
	switch config.Identity.Role {
	default:
		opts, err := basenode.OptionsFromRepo(rep)
		if err != nil {
			return err
		}

		password := cctx.String("password")
		opts = append(opts, basenode.SetPassword(password))

		node, err = provider.New(cctx.Context, opts...)
		if err != nil {
			return err
		}
	}

	// Start the node
	if err := node.Start(); err != nil {
		return err
	}

	return node.RunDaemon()
}
