package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"

	"github.com/memoio/go-mefs-v2/app/cmd"
)

// full compatible with ipfs
func main() {
	local := []*cli.Command{
		DaemonCmd,
		cmd.InitCmd,
		cmd.AuthCmd,
		cmd.WalletCmd,
	}
	//no ipfs commands
	app := &cli.App{
		Name:                 "mefs-keeper",
		Usage:                "Memoriae decentralized storage network node",
		Version:              "1.0.0",
		EnableBashCompletion: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    cmd.FlagNodeRepo,
				EnvVars: []string{"MEMO_PATH"},
				Value:   "~/.memo",
				Usage:   "Specify memoriae path.",
			},
			&cli.StringFlag{
				Name:  cmd.FlagRoleType,
				Value: "keeper",
				Usage: "set role type.",
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
