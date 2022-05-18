package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"

	"github.com/memoio/go-mefs-v2/app/cmd"
	lfscmd "github.com/memoio/go-mefs-v2/app/cmd/lfs"
	"github.com/memoio/go-mefs-v2/app/minit"
	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/pb"
)

func main() {
	local := make([]*cli.Command, 0, len(cmd.CommonCmd))
	local = append(local, cmd.CommonCmd...)
	local = append(local, lfscmd.LfsCmd)
	local = append(local, OrderCmd)
	local = append(local, GatewayCmd)

	app := &cli.App{
		Name:                 "mefs-user",
		Usage:                "Memoriae decentralized storage network node",
		Version:              build.UserVersion(),
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

	err := app.Run(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n\n", err) // nolint:errcheck
		os.Exit(1)
	}
}
