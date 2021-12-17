package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"

	"github.com/memoio/go-mefs-v2/app/cmd"
	"github.com/memoio/go-mefs-v2/app/minit"
	logging "github.com/memoio/go-mefs-v2/lib/log"
)

var logger = logging.Logger("mefs-provider")

func main() {
	local := []*cli.Command{
		DaemonCmd,
		cmd.InitCmd,
		cmd.AuthCmd,
		cmd.WalletCmd,
		cmd.StateCmd,
		cmd.NetCmd,
	}

	app := &cli.App{
		Name:                 "mefs-provider",
		Usage:                "Memoriae decentralized storage network node",
		Version:              "1.0.0",
		EnableBashCompletion: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    cmd.FlagNodeRepo,
				EnvVars: []string{"MEFS_PATH"},
				Value:   "~/.memo-provider",
				Usage:   "Specify memoriae path.",
			},
			&cli.StringFlag{
				Name:  cmd.FlagRoleType,
				Value: "provider",
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
