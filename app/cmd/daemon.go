package cmd

import (
	_ "net/http/pprof"
	"strings"

	"github.com/urfave/cli/v2"

	"github.com/memoio/go-mefs-v2/app/minit"
	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/repo"
	"github.com/memoio/go-mefs-v2/service/keeper"
	"github.com/memoio/go-mefs-v2/service/provider"
	"github.com/memoio/go-mefs-v2/service/user"
	"github.com/memoio/go-mefs-v2/submodule/connect/settle"
	basenode "github.com/memoio/go-mefs-v2/submodule/node"
)

const (
	apiAddrKwd   = "api"
	swarmPortKwd = "swarm-port"
	pwKwd        = "password"
	groupKwd     = "group"
)

var DaemonCmd = &cli.Command{
	Name:  "daemon",
	Usage: "Run a network-connected Memoriae keeper.",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  pwKwd,
			Usage: "password for asset private key",
			Value: "memoriae",
		},
		&cli.StringFlag{
			Name:  apiAddrKwd,
			Usage: "set the api addr to use",
			Value: "/ip4/127.0.0.1/tcp/8001",
		},
		&cli.StringFlag{
			Name:  swarmPortKwd,
			Usage: "set the swarm port to use",
			Value: "7001",
		},
		&cli.Uint64Flag{
			Name:  groupKwd,
			Usage: "set the group number",
			Value: 1,
		},
	},
	Action: func(cctx *cli.Context) error {
		return daemonFunc(cctx)
	},
}

func daemonFunc(cctx *cli.Context) (_err error) {
	logger.Info("Initializing daemon...")

	ctx := cctx.Context
	minit.StartMetrics()

	minit.PrintVersion()

	stopFunc, err := minit.ProfileIfEnabled()
	if err != nil {
		return err
	}
	defer stopFunc()

	repoDir := cctx.String(FlagNodeRepo)
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

	pwd := cctx.String("password")
	opts, err := basenode.OptionsFromRepo(rep)
	if err != nil {
		return err
	}
	opts = append(opts, basenode.SetPassword(pwd))

	laddr, err := address.NewFromString(config.Wallet.DefaultAddress)
	if err != nil {
		return err
	}

	ki, err := rep.KeyStore().Get(laddr.String(), pwd)
	if err != nil {
		return err
	}

	var node minit.Node
	switch cctx.String(FlagRoleType) {
	case pb.RoleInfo_Keeper.String():
		rid, gid, err := settle.Register(ctx, ki.SecretKey, pb.RoleInfo_Keeper, cctx.Uint64(groupKwd))
		if err != nil {
			return err
		}

		opts = append(opts, basenode.SetRoleID(rid))
		opts = append(opts, basenode.SetGroupID(gid))

		node, err = keeper.New(ctx, opts...)
		if err != nil {
			return err
		}
	case pb.RoleInfo_Provider.String():
		rid, gid, err := settle.Register(ctx, ki.SecretKey, pb.RoleInfo_Provider, cctx.Uint64(groupKwd))
		if err != nil {
			return err
		}

		opts = append(opts, basenode.SetRoleID(rid))
		opts = append(opts, basenode.SetGroupID(gid))

		node, err = provider.New(ctx, opts...)
		if err != nil {
			return err
		}
	case pb.RoleInfo_User.String():
		rid, gid, err := settle.Register(ctx, ki.SecretKey, pb.RoleInfo_User, cctx.Uint64(groupKwd))
		if err != nil {
			return err
		}

		opts = append(opts, basenode.SetRoleID(rid))
		opts = append(opts, basenode.SetGroupID(gid))

		node, err = user.New(ctx, opts...)
		if err != nil {
			return err
		}
	default:
		rid, gid, err := settle.Register(ctx, ki.SecretKey, pb.RoleInfo_Unknown, 1)
		if err != nil {
			return err
		}

		opts = append(opts, basenode.SetRoleID(rid))
		opts = append(opts, basenode.SetGroupID(gid))

		node, err = basenode.New(ctx, opts...)
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
