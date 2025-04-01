package cmd

import (
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/memoio/go-mefs-v2/api/client"
)

var logCmd = &cli.Command{
	Name:  "log",
	Usage: "Manage logging",
	Subcommands: []*cli.Command{
		logSetLevel,
	},
}

var logSetLevel = &cli.Command{
	Name:      "setLevel",
	Usage:     "Set log level",
	ArgsUsage: "[level (debug/info/warn/error)]",
	Action: func(cctx *cli.Context) error {
		repoDir := cctx.String(FlagNodeRepo)
		addr, headers, err := client.GetMemoClientInfo(repoDir)
		if err != nil {
			return err
		}

		api, closer, err := client.NewGenericNode(cctx.Context, addr, headers)
		if err != nil {
			return err
		}
		defer closer()

		if !cctx.Args().Present() {
			return fmt.Errorf("level is required")
		}

		err = api.LogSetLevel(cctx.Context, cctx.Args().First())
		if err != nil {
			return err
		}

		return nil
	},
}
