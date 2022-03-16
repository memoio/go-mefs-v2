package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/memoio/go-mefs-v2/api/client"
	"github.com/memoio/go-mefs-v2/app/cmd"
	"github.com/memoio/go-mefs-v2/lib/utils"

	"github.com/mgutz/ansi"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var OrderCmd = &cli.Command{
	Name:  "order",
	Usage: "Interact with order",
	Subcommands: []*cli.Command{
		orderListJobCmd,
		orderListPayCmd,
		orderGetCmd,
		orderDetailCmd,
		orderListProvidersCmd,
	},
}

var orderListProvidersCmd = &cli.Command{
	Name:  "proList",
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
			fmt.Printf("proID: %d, ready: %t, stop %t, netID: %s", oi.ID, oi.Ready, oi.InStop, oi.PeerID)
		}

		return nil
	},
}

var orderListJobCmd = &cli.Command{
	Name:  "jobList",
	Usage: "list jobs of all pros",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "verbose",
			Usage: "filter output",
			Value: false,
		},
	},
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

		verbose := cctx.Bool("verbose")

		for _, oi := range ois {
			if !verbose && oi.InStop {
				continue
			}
			fmt.Printf("proID: %d, jobs: %d, order: %d %s %s, seq: %d %s, ready: %t, stop: %t, avail: %s\n", oi.ID, oi.Jobs, oi.Nonce, ansi.Color(oi.OrderState, "green"), time.Unix(int64(oi.OrderTime), 0).Format(utils.SHOWTIME), oi.SeqNum, ansi.Color(oi.SeqState, "green"), oi.Ready, oi.InStop, time.Unix(int64(oi.AvailTime), 0).Format(utils.SHOWTIME))
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

		for _, oi := range ois {
			fmt.Printf("proID: %d, size: %d, confirmed: %d, onChain: %d, need pay : %d, paid %d\n", oi.ID, oi.Size, oi.ConfirmSize, oi.OnChainSize, oi.NeedPay, oi.Paid)
		}

		return nil
	},
}

var orderGetCmd = &cli.Command{
	Name:      "get",
	Usage:     "get order info of one provider",
	ArgsUsage: "[provider index required]",
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return xerrors.Errorf("need amount")
		}
		pid, err := strconv.ParseUint(cctx.Args().First(), 10, 0)
		if err != nil {
			return xerrors.Errorf("parsing 'amount' argument: %w", err)
		}

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

		oi, err := api.OrderGetJobInfoAt(cctx.Context, pid)
		if err != nil {
			return err
		}

		ns := api.StateGetOrderState(cctx.Context, api.SettleGetRoleID(cctx.Context), pid)

		si, err := api.SettleGetStoreInfo(cctx.Context, api.SettleGetRoleID(cctx.Context), pid)
		if err != nil {
			return err
		}

		fmt.Printf("proID: %d, jobs: %d, order: %d %d %d %s %s, seq: %d %s, ready: %t, stop: %t, avail: %s\n", oi.ID, oi.Jobs, si.Nonce, ns.Nonce, oi.Nonce, ansi.Color(oi.OrderState, "green"), time.Unix(int64(oi.OrderTime), 0).Format(utils.SHOWTIME), oi.SeqNum, ansi.Color(oi.SeqState, "green"), oi.Ready, oi.InStop, time.Unix(int64(oi.AvailTime), 0).Format(utils.SHOWTIME))

		return nil
	},
}

var orderDetailCmd = &cli.Command{
	Name:      "detail",
	Usage:     "get detail order seq info of one provider",
	ArgsUsage: "[provider index required] [order nonce] [seq number]",
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 3 {
			return xerrors.Errorf("need three parameters")
		}

		pid, err := strconv.ParseUint(cctx.Args().First(), 10, 0)
		if err != nil {
			return xerrors.Errorf("parsing 'pro indec' argument: %w", err)
		}

		nc, err := strconv.ParseUint(cctx.Args().Get(1), 10, 0)
		if err != nil {
			return xerrors.Errorf("parsing 'nonce' argument: %w", err)
		}

		sn, err := strconv.ParseUint(cctx.Args().Get(2), 10, 0)
		if err != nil {
			return xerrors.Errorf("parsing 'seq number' argument: %w", err)
		}

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

		oi, err := api.OrderGetDetail(cctx.Context, pid, nc, uint32(sn))
		if err != nil {
			return err
		}

		fmt.Printf("proID: %d, nonce: %d, seqNum: %d, size: %d, segment: %d\n", oi.ProID, oi.Nonce, oi.SeqNum, oi.Size, oi.Segments.Len())

		for _, seg := range oi.Segments {
			fmt.Println("seg: ", seg)
		}

		return nil
	},
}
