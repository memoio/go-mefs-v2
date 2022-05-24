package cmd

import (
	_ "net/http/pprof"
	"os"
	"strconv"
	"strings"

	"github.com/urfave/cli/v2"

	"github.com/memoio/go-mefs-v2/api/client"
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
	apiAddrKwd    = "api"
	swarmPortKwd  = "swarm-port"
	pwKwd         = "password"
	groupKwd      = "group"
	MEMO_PASSWORD = "MEMO_PASSWORD"
)

var daemonCmd = &cli.Command{
	Name:  "daemon",
	Usage: "Run a network-connected Memoriae node.",

	Subcommands: []*cli.Command{
		daemonStartCmd,
		daemonStopCmd,
	},
}

var daemonStartCmd = &cli.Command{
	Name:  "start",
	Usage: "Start a running mefs daemon",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    pwKwd,
			Aliases: []string{"pwd"},
			Usage:   "password for asset private key",
			Value:   "memoriae",
		},
		&cli.StringFlag{
			Name:  apiAddrKwd,
			Usage: "set the api addr to use",
			Value: "/ip4/127.0.0.1/tcp/5001",
		},
		&cli.StringFlag{
			Name:    "secretKey",
			Aliases: []string{"sk"},
			Usage:   "secret key to use if not init",
			Value:   "",
		},
		&cli.StringFlag{
			Name:  swarmPortKwd,
			Usage: "set the swarm port to use",
			Value: "4001",
		},
		&cli.Uint64Flag{
			Name:  groupKwd,
			Usage: "set the group number",
			Value: 0,
		},
		&cli.Uint64Flag{
			Name:  "price",
			Usage: "segment price",
			Value: 0,
		},
		&cli.BoolFlag{
			Name:  "secureAPI",
			Usage: "API is secure or insecure",
			Value: true,
		},
	},
	Action: func(cctx *cli.Context) error {
		return daemonStartFunc(cctx)
	},
}

var daemonStopCmd = &cli.Command{
	Name:  "stop",
	Usage: "Stop a running mefs daemon",
	Action: func(cctx *cli.Context) error {
		return daemonStopFunc(cctx)
	},
}

// create a node with repo data and start it
func daemonStartFunc(cctx *cli.Context) (_err error) {
	ctx := cctx.Context
	minit.StartMetrics()

	minit.PrintVersion()

	logger.Info("Initializing daemon...")

	stopFunc, err := minit.ProfileIfEnabled()
	if err != nil {
		return err
	}
	defer stopFunc()

	repoDir := cctx.String(FlagNodeRepo)
	// generate a repo from repoDir
	rep, err := repo.NewFSRepo(repoDir, nil)
	if err != nil {
		return err
	}

	defer rep.Close()

	pwd := cctx.String(pwKwd)
	if os.Getenv(MEMO_PASSWORD) != "" {
		pwd = os.Getenv(MEMO_PASSWORD)
	}

	sk := cctx.String("sk")
	if sk != "" {
		err = minit.Create(cctx.Context, rep, pwd, sk)
		if err != nil {
			logger.Errorf("Fail starting node, reason: %s", err)
			return err
		}
	}

	// handle cfg
	cfg := rep.Config()

	if swarmPort := cctx.String(swarmPortKwd); swarmPort != "" {
		changed := make([]string, 0, len(cfg.Net.Addresses))
		for _, swarmAddr := range cfg.Net.Addresses {
			strs := strings.Split(swarmAddr, "/")
			for i, str := range strs {
				if str == "tcp" || str == "udp" {
					strs[i+1] = swarmPort
				}
			}
			changed = append(changed, strings.Join(strs, "/"))
		}
		cfg.Net.Addresses = changed
	}

	apiAddr := cctx.String(apiAddrKwd)
	if apiAddr != "" {
		cfg.API.Address = apiAddr
	}

	pr := cctx.Uint64("price")
	if pr > 0 {
		cfg.Order.Price = pr
	}

	rep.ReplaceConfig(cfg)

	opts, err := basenode.OptionsFromRepo(rep)
	if err != nil {
		return err
	}
	opts = append(opts, basenode.SetPassword(pwd))

	laddr, err := address.NewFromString(cfg.Wallet.DefaultAddress)
	if err != nil {
		return err
	}

	ki, err := rep.KeyStore().Get(laddr.String(), pwd)
	if err != nil {
		return err
	}

	var node minit.Node
	// create the node with opts above
	switch cctx.String(FlagRoleType) {
	case pb.RoleInfo_Keeper.String():
		rid, gid, err := settle.Register(ctx, cfg.Contract.EndPoint, cfg.Contract.RoleContract, ki.SecretKey, pb.RoleInfo_Keeper, cctx.Uint64(groupKwd))
		if err != nil {
			return err
		}

		cfg.Identity.Role = "keeper"
		cfg.Identity.Group = strconv.Itoa(int(gid))

		rep.ReplaceConfig(cfg)

		opts = append(opts, basenode.SetRoleID(rid))
		opts = append(opts, basenode.SetGroupID(gid))

		node, err = keeper.New(ctx, opts...)
		if err != nil {
			return err
		}
	case pb.RoleInfo_Provider.String():
		rid, gid, err := settle.Register(ctx, cfg.Contract.EndPoint, cfg.Contract.RoleContract, ki.SecretKey, pb.RoleInfo_Provider, cctx.Uint64(groupKwd))
		if err != nil {
			return err
		}

		cfg.Identity.Role = "provider"
		cfg.Identity.Group = strconv.Itoa(int(gid))
		rep.ReplaceConfig(cfg)

		opts = append(opts, basenode.SetRoleID(rid))
		opts = append(opts, basenode.SetGroupID(gid))

		node, err = provider.New(ctx, opts...)
		if err != nil {
			return err
		}
	case pb.RoleInfo_User.String():
		rid, gid, err := settle.Register(ctx, cfg.Contract.EndPoint, cfg.Contract.RoleContract, ki.SecretKey, pb.RoleInfo_User, cctx.Uint64(groupKwd))
		if err != nil {
			return err
		}

		cfg.Identity.Role = "user"
		cfg.Identity.Group = strconv.Itoa(int(gid))
		rep.ReplaceConfig(cfg)

		opts = append(opts, basenode.SetRoleID(rid))
		opts = append(opts, basenode.SetGroupID(gid))

		node, err = user.New(ctx, opts...)
		if err != nil {
			return err
		}
	default:
		rid, gid, err := settle.Register(ctx, cfg.Contract.EndPoint, cfg.Contract.RoleContract, ki.SecretKey, pb.RoleInfo_Unknown, cctx.Uint64(groupKwd))
		if err != nil {
			return err
		}

		cfg.Identity.Role = "unkown"
		cfg.Identity.Group = strconv.Itoa(int(gid))
		rep.ReplaceConfig(cfg)

		opts = append(opts, basenode.SetRoleID(rid))
		opts = append(opts, basenode.SetGroupID(gid))

		node, err = basenode.New(ctx, opts...)
		if err != nil {
			return err
		}
	}

	// Start the node
	err = node.Start(cctx.Bool("secureAPI"))
	if err != nil {
		return err
	}

	return node.RunDaemon()
}

// stop a node
func daemonStopFunc(cctx *cli.Context) (_err error) {
	repoDir := cctx.String(FlagNodeRepo)
	addr, headers, err := client.GetMemoClientInfo(repoDir)
	if err != nil {
		return err
	}

	napi, closer, err := client.NewGenericNode(cctx.Context, addr, headers)
	if err != nil {
		return err
	}
	defer closer()

	err = napi.Shutdown(cctx.Context)
	if err != nil {
		return err
	}

	return nil
}
