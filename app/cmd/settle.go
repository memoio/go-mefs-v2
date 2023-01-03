package cmd

import (
	"fmt"
	"math/big"
	"time"

	"github.com/memoio/go-mefs-v2/api/client"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var settleCmd = &cli.Command{
	Name:  "settle",
	Usage: "Interact with settlement chain",
	Subcommands: []*cli.Command{
		settleSetDescCmd,
		settleWithdrawCmd,
		pledgeAddCmd,
		pledgeGetCmd,
		pledgeWithdrawCmd,
		pledgeRewardWithdrawCmd,
		settleQuitRoleCmd,
		settleAlterPayeeCmd,
	},
}

var pledgeGetCmd = &cli.Command{
	Name:  "pledgeGet",
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
	Name:      "pledgeAdd",
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
	Name:      "pledgeWithdraw",
	Usage:     "move pledge value to fs, then call settle withdraw",
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

		rid := api.SettleGetRoleID(cctx.Context)

		fi, err := api.SettleGetBalanceInfo(cctx.Context, rid)
		if err != nil {
			return err
		}

		pi, err := api.SettleGetPledgeInfo(cctx.Context, rid)
		if err != nil {
			return err
		}

		fmt.Printf("Before Withdraw: %s, %s (in fs), %s (in pledge), %s (total pledge), %s (total pledge + reward), %s (current pledge)\n", types.FormatMemo(fi.ErcValue), types.FormatMemo(fi.FsValue), types.FormatMemo(pi.Value), types.FormatMemo(pi.Total), types.FormatMemo(pi.ErcTotal), types.FormatMemo(pi.Last))

		if cctx.Args().Present() {
			val, err := types.ParsetValue(cctx.Args().First())
			if err != nil {
				return xerrors.Errorf("parsing 'amount' argument: %w", err)
			}
			pi.Value.Set(val)
		}

		fmt.Println("Withdraw: ", types.FormatMemo(pi.Value))

		err = api.SettlePledgeWithdraw(cctx.Context, pi.Value)
		if err != nil {
			return err
		}

		fi, err = api.SettleGetBalanceInfo(cctx.Context, rid)
		if err != nil {
			return err
		}

		pi, err = api.SettleGetPledgeInfo(cctx.Context, rid)
		if err != nil {
			return err
		}

		fmt.Printf("After Withdraw: %s, %s (in fs), %s (in pledge), %s (total pledge), %s (total pledge + reward), %s (current pledge)\n", types.FormatMemo(fi.ErcValue), types.FormatMemo(fi.FsValue), types.FormatMemo(pi.Value), types.FormatMemo(pi.Total), types.FormatMemo(pi.ErcTotal), types.FormatMemo(pi.Last))

		return nil
	},
}

var pledgeRewardWithdrawCmd = &cli.Command{
	Name:      "pledgeRewardWithdraw",
	Usage:     "move pledge reward value to fs, then call settle withdraw",
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

		rid := api.SettleGetRoleID(cctx.Context)

		fi, err := api.SettleGetBalanceInfo(cctx.Context, rid)
		if err != nil {
			return err
		}

		pi, err := api.SettleGetPledgeInfo(cctx.Context, rid)
		if err != nil {
			return err
		}

		fmt.Printf("Before Withdraw: %s, %s (in fs), %s (in pledge), %s (total pledge), %s (total pledge + reward), %s (current reward)\n", types.FormatMemo(fi.ErcValue), types.FormatMemo(fi.FsValue), types.FormatMemo(pi.Value), types.FormatMemo(pi.Total), types.FormatMemo(pi.ErcTotal), types.FormatMemo(pi.CurReward))

		if cctx.Args().Present() {
			val, err := types.ParsetValue(cctx.Args().First())
			if err != nil {
				return xerrors.Errorf("parsing 'amount' argument: %w", err)
			}
			pi.Value.Set(val)
		}

		fmt.Println("Withdraw: ", types.FormatMemo(pi.Value))

		err = api.SettlePledgeRewardWithdraw(cctx.Context, pi.Value)
		if err != nil {
			return err
		}

		fi, err = api.SettleGetBalanceInfo(cctx.Context, rid)
		if err != nil {
			return err
		}

		pi, err = api.SettleGetPledgeInfo(cctx.Context, rid)
		if err != nil {
			return err
		}

		fmt.Printf("After Withdraw: %s, %s (in fs), %s (in pledge), %s (total pledge), %s (total pledge + reward), %s (current reward)\n", types.FormatMemo(fi.ErcValue), types.FormatMemo(fi.FsValue), types.FormatMemo(pi.Value), types.FormatMemo(pi.Total), types.FormatMemo(pi.ErcTotal), types.FormatMemo(pi.CurReward))

		return nil
	},
}

var settleSetDescCmd = &cli.Command{
	Name:      "setDesc",
	Usage:     "Set description for a node. Especially for providers, if desc is set to 'cloud', they will be selected as dc in buckets preferentially.",
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

var settleWithdrawCmd = &cli.Command{
	Name:      "withdraw",
	Usage:     "withdraw memo from fs",
	ArgsUsage: "[[amount (Memo / NanoMemo / AttoMemo) optional, otherwise withdraw max available]]",
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

		rid := api.SettleGetRoleID(cctx.Context)

		pi, err := api.SettleGetBalanceInfo(cctx.Context, rid)
		if err != nil {
			return err
		}

		fmt.Printf("Before Withdraw: %s, %s (in fs) \n", types.FormatMemo(pi.ErcValue), types.FormatMemo(pi.FsValue))

		val := new(big.Int).Set(pi.FsValue)

		if cctx.Args().First() != "" {
			val, err = types.ParsetValue(cctx.Args().First())
			if err != nil {
				return xerrors.Errorf("parsing 'amount' argument: %w", err)
			}
		}

		fmt.Println("Withdraw: ", types.FormatMemo(val))

		err = api.SettleWithdraw(cctx.Context, val)
		if err != nil {
			return err
		}

		time.Sleep(10 * time.Second)

		pi, err = api.SettleGetBalanceInfo(cctx.Context, rid)
		if err != nil {
			return err
		}

		fmt.Printf("After Withdraw: %s, %s (in fs) \n", types.FormatMemo(pi.ErcValue), types.FormatMemo(pi.FsValue))

		return nil
	},
}

var settleQuitRoleCmd = &cli.Command{
	Name:  "quitRole",
	Usage: "change its state to inactive, this op is invocatable and daemon will fail at next start",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "really-do-it",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Bool("really-do-it") {
			return xerrors.Errorf("need --really-do-it, this op is invocatable and daemon will fail at next start, make sure before you do this")
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

		err = api.SettleQuitRole(cctx.Context)
		if err != nil {
			return err
		}

		fmt.Println("quit role successfully")

		return nil
	},
}

var settleAlterPayeeCmd = &cli.Command{
	Name:      "alterPayee",
	Usage:     "alter current payee to a new one, need to be comfirmed by new payee to finish.",
	ArgsUsage: "[address(0x...), required]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "really-do-it",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Bool("really-do-it") {
			return xerrors.Errorf("need --really-do-it")
		}

		if !cctx.Args().Present() {
			return xerrors.Errorf("need payee address")
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

		err = api.SettleAlterPayee(cctx.Context, cctx.Args().First())
		if err != nil {
			return err
		}

		fmt.Println("successfully alter payee to: ", cctx.Args().First())
		fmt.Println("next step: confirm payee using", cctx.Args().First())

		return nil
	},
}
