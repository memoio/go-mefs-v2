package main

import (
	"fmt"
	"sort"
	"time"

	"github.com/memoio/go-mefs-v2/api/client"
	"github.com/memoio/go-mefs-v2/app/cmd"
	"github.com/memoio/go-mefs-v2/lib/utils"
	"github.com/modood/table"
	"github.com/urfave/cli/v2"
)

var OrderCmd = &cli.Command{
	Name:  "order",
	Usage: "Interact with order",
	Subcommands: []*cli.Command{
		orderListCmd,
		orderListPayCmd,
	},
}

var orderListCmd = &cli.Command{
	Name:  "list",
	Usage: "list all users",
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

var orderListPayCmd = &cli.Command{
	Name:  "payList",
	Usage: "list pay infos all pros",
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

		ois, err := api.OrderGetPayInfo(cctx.Context)
		if err != nil {
			return err
		}

		type outPutInfo struct {
			ID          uint64
			Size        uint64
			ConfirmSize uint64
			OnChainSize uint64
			NeedPay     string
			Paid        string
		}
		outPut := make([]outPutInfo, 0)
		var tmp outPutInfo

		for _, oi := range ois {
			tmp = outPutInfo{oi.ID, oi.Size, oi.ConfirmSize, oi.OnChainSize, oi.NeedPay.String(), oi.Paid.String()}
			outPut = append(outPut, tmp)
		}
		sort.Slice(outPut, func(i, j int) bool { return outPut[i].ID < outPut[j].ID })
		table.Output(outPut)

		return nil
	},
}
