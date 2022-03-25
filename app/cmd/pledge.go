package cmd

import (
	"fmt"

	"github.com/memoio/go-mefs-v2/api/client"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var pledgeCmd = &cli.Command{
	Name:  "pledge",
	Usage: "Interact with pledge",
	Subcommands: []*cli.Command{
		pledgeAddCmd,
		pledgeGetCmd,
		pledgeWithdrawCmd,
	},
}

var pledgeGetCmd = &cli.Command{
	Name:  "get",
	Usage: "get pledge information",
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

		pi, err := api.SettleGetPledgeInfo(cctx.Context, api.SettleGetRoleID(cctx.Context))
		if err != nil {
			return err
		}
		fmt.Printf("Pledge: %s, %s (total pledge), %s (total pledge + reward) \n", types.FormatMemo(pi.Value), types.FormatMemo(pi.Total), types.FormatMemo(pi.ErcTotal))

		return nil
	},
}

var pledgeAddCmd = &cli.Command{
	Name:      "add",
	Usage:     "add pledge value",
	ArgsUsage: "[amount (Memo / NanoMemo / AttoMemo) required]",
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return xerrors.Errorf("need amount")
		}
		val, err := types.ParsetValue(cctx.Args().First())
		if err != nil {
			return xerrors.Errorf("parsing 'amount' argument: %w", err)
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

		pi, err := api.SettleGetPledgeInfo(cctx.Context, api.SettleGetRoleID(cctx.Context))
		if err != nil {
			return err
		}
		fmt.Printf("Before Pledge: %s, %s (total pledge), %s (total pledge + reward) \n", types.FormatMemo(pi.Value), types.FormatMemo(pi.Total), types.FormatMemo(pi.ErcTotal))

		fmt.Println("Pledge: ", types.FormatMemo(val))

		err = api.SettlePledge(cctx.Context, val)
		if err != nil {
			return err
		}

		pi, err = api.SettleGetPledgeInfo(cctx.Context, api.SettleGetRoleID(cctx.Context))
		if err != nil {
			return err
		}
		fmt.Printf("After Pledge: %s, %s (total pledge), %s (total pledge + reward) \n", types.FormatMemo(pi.Value), types.FormatMemo(pi.Total), types.FormatMemo(pi.ErcTotal))

		return nil
	},
}

var pledgeWithdrawCmd = &cli.Command{
	Name:      "withdraw",
	Usage:     "withdraw pledge value",
	ArgsUsage: "[amount (Memo / NanoMemo / AttoMemo) optional, otherwise withdraw max available]",
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

		pi, err := api.SettleGetPledgeInfo(cctx.Context, api.SettleGetRoleID(cctx.Context))
		if err != nil {
			return err
		}
		fmt.Printf("Before Withdraw: %s, %s (total pledge), %s (total pledge + reward) \n", types.FormatMemo(pi.Value), types.FormatMemo(pi.Total), types.FormatMemo(pi.ErcTotal))

		if cctx.Args().Present() {
			val, err := types.ParsetValue(cctx.Args().First())
			if err != nil {
				return xerrors.Errorf("parsing 'amount' argument: %w", err)
			}
			pi.Value.Set(val)
		}

		fmt.Println("Withdraw: ", types.FormatMemo(pi.Value))

		err = api.SettleCanclePledge(cctx.Context, pi.Value)
		if err != nil {
			return err
		}

		pi, err = api.SettleGetPledgeInfo(cctx.Context, api.SettleGetRoleID(cctx.Context))
		if err != nil {
			return err
		}
		fmt.Printf("After Withdraw: %s, %s (total pledge), %s (total pledge + reward) \n", types.FormatMemo(pi.Value), types.FormatMemo(pi.Total), types.FormatMemo(pi.ErcTotal))

		return nil
	},
}
