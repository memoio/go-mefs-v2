package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"

	"github.com/memoio/go-mefs-v2/app/cmd"
	lfscmd "github.com/memoio/go-mefs-v2/app/cmd/lfs"
	"github.com/memoio/go-mefs-v2/app/minit"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/pb"
)

var logger = logging.Logger("mefs-user")

func main() {
	local := []*cli.Command{
		cmd.DaemonCmd,
		cmd.InitCmd,
		cmd.AuthCmd,
		cmd.WalletCmd,
		cmd.RoleCmd,
		cmd.NetCmd,
		lfscmd.LfsCmd,
		cmd.ConfigCmd,
	}

	app := &cli.App{
		Name:                 "mefs-user",
		Usage:                "Memoriae decentralized storage network node",
		Version:              "1.0.0",
		EnableBashCompletion: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    cmd.FlagNodeRepo,
				EnvVars: []string{"MEFS_PATH"},
				Value:   "~/.memo-user",
				Usage:   "Specify memoriae path.",
			},
			&cli.StringFlag{
				Name:  cmd.FlagRoleType,
				Value: pb.RoleInfo_User.String(),
				Usage: "set role type.",
			},
			&cli.StringFlag{
				Name:  minit.EnvEnableProfiling,
				Value: "enable",
				Usage: "enable cpu profile",
			},
		},

		Commands: local,
	}

	app.Setup()

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n\n", err) // nolint:errcheck
		os.Exit(1)
	}
}
