package main

import (
	"fmt"
	_ "net/http/pprof"
	"os"

	"github.com/urfave/cli/v2"

	"github.com/memoio/go-mefs-v2/app/cmd"
	logging "github.com/memoio/go-mefs-v2/lib/log"
)

var logger = logging.Logger("mefs")

// full compatible with ipfs
func main() {
	local := make([]*cli.Command, 0, len(cmd.CommonCmd))
	local = append(local, cmd.CommonCmd...)

	app := &cli.App{
		Name:                 "mefs",
		Usage:                "Memoriae decentralized storage network node",
		Version:              "1.0.0",
		EnableBashCompletion: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    cmd.FlagNodeRepo,
				EnvVars: []string{"MEFS_PATH"},
				Value:   "~/.memo",
				Usage:   "Specify memoriae path.",
			},
			&cli.StringFlag{
				Name:  cmd.FlagRoleType,
				Value: "unknown",
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
