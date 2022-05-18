package main

import (
	"fmt"
	_ "net/http/pprof"
	"os"

	"github.com/urfave/cli/v2"

	"github.com/memoio/go-mefs-v2/app/cmd"
	"github.com/memoio/go-mefs-v2/build"
)

// full compatible with ipfs
func main() {
	local := make([]*cli.Command, 0, len(cmd.CommonCmd)+1)
	local = append(local, cmd.CommonCmd...)
	local = append(local, cmd.TransferCmd)

	app := &cli.App{
		Name:                 "mefs",
		Usage:                "Memoriae decentralized storage network node",
		Version:              build.UserVersion(),
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

	err := app.Run(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n\n", err) // nolint:errcheck
		os.Exit(1)
	}
}
