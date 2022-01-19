package main

import (
	"fmt"

	"github.com/memoio/go-mefs-v2/api/client"
	"github.com/memoio/go-mefs-v2/app/cmd"
	"github.com/urfave/cli/v2"
)

var OrderCmd = &cli.Command{
	Name:  "order",
	Usage: "Interact with pledge",
	Subcommands: []*cli.Command{
		orderListCmd,
	},
}

var orderListCmd = &cli.Command{
	Name:  "list",
	Usage: "list all pros",
	Action: func(cctx *cli.Context) error {
		repoDir := cctx.String(cmd.FlagNodeRepo)
		addr, headers, err := client.GetMemoClientInfo(repoDir)
		if err != nil {
			return err
		}

		api, closer, err := client.NewUserNode(cctx.Context, addr, headers)
		if err != nil {
			return err
		}
		defer closer()

		ois, err := api.OrderGetInfo(cctx.Context)
		if err != nil {
			return err
		}

		for _, oi := range ois {
			fmt.Println(oi)
		}

		return nil
	},
}
