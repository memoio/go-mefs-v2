package main

import (
	"fmt"
	"time"

	"github.com/memoio/go-mefs-v2/api/client"
	"github.com/memoio/go-mefs-v2/app/cmd"
	"github.com/memoio/go-mefs-v2/lib/utils"
	"github.com/urfave/cli/v2"
)

var OrderCmd = &cli.Command{
	Name:  "order",
	Usage: "Interact with order",
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

		ois, err := api.OrderGetJobInfo(cctx.Context)
		if err != nil {
			return err
		}

		for _, oi := range ois {
			fmt.Printf("userID: %d, order: %d %s, seq: %d %s, ready: %t, pause: %t, avail: %s\n", oi.ID, oi.Nonce, oi.OrderState, oi.SeqNum, oi.SeqState, oi.Ready, oi.InStop, time.Unix(int64(oi.AvailTime), 0).Format(utils.SHOWTIME))
		}

		return nil
	},
}
