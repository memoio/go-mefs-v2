package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"

	generic_cmd "github.com/memoio/go-mefs-v2/app/generic"
)

// full compatible with ipfs
func main() {
	local := []*cli.Command{
		DaemonCmd,
		generic_cmd.InitCmd,
	}
	//no ipfs commands
	app := &cli.App{
		Name:                 "mefs",
		Usage:                "Memoriae decentralized storage network node",
		Version:              "1.0.0",
		EnableBashCompletion: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    generic_cmd.FlagNodeRepo,
				EnvVars: []string{"MEMO_PATH"},
				Value:   "~/.memo", // TODO: Consider XDG_DATA_HOME
				Usage:   "Specify memoriae path.",
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
