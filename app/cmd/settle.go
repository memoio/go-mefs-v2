package cmd

import (
	"github.com/memoio/go-mefs-v2/api/client"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var settleCmd = &cli.Command{
	Name:  "settle",
	Usage: "Interact with settlement chain",
	Subcommands: []*cli.Command{
		settleSetDescCmd,
	},
}

var settleSetDescCmd = &cli.Command{
	Name:      "setDesc",
	Usage:     "set desc",
	ArgsUsage: "[desc required]",
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return xerrors.Errorf("need desc")
		}

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

		err = api.SettleSetDesc(cctx.Context, []byte(cctx.Args().First()))
		if err != nil {
			return err
		}

		return nil
	},
}
