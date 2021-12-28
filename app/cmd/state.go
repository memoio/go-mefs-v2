package cmd

import (
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/memoio/go-mefs-v2/api/client"
	"github.com/memoio/go-mefs-v2/lib/types"
)

var StateCmd = &cli.Command{
	Name:  "state",
	Usage: "Interact with state manager",
	Subcommands: []*cli.Command{
		statePostIncomeCmd,
		statePayCmd,
		stateWithdrawCmd,
	},
}

var statePostIncomeCmd = &cli.Command{
	Name:  "post",
	Usage: "list post",
	Action: func(cctx *cli.Context) error {
		repoDir := cctx.String(FlagNodeRepo)
		addr, headers, err := client.GetMemoClientInfo(repoDir)
		if err != nil {
			return err
		}

		napi, closer, err := client.NewGenericNode(cctx.Context, addr, headers)
		if err != nil {
			return err
		}
		defer closer()

		nid, err := napi.RoleSelf(cctx.Context)
		if err != nil {
			return err
		}

		fmt.Println("post income: ", nid.ID)

		users := napi.GetUsersForPro(cctx.Context, nid.ID)
		fmt.Println("post income: ", nid.ID, users)

		for _, uid := range users {
			pi := napi.GetPostIncome(cctx.Context, uid, nid.ID)
			fmt.Printf("post income: proID %d, userID %d, income: %s \n", nid.ID, uid, types.FormatWei(pi.Value))
		}

		return nil
	},
}

var statePayCmd = &cli.Command{
	Name:  "pay",
	Usage: "list pay",
	Action: func(cctx *cli.Context) error {
		repoDir := cctx.String(FlagNodeRepo)
		addr, headers, err := client.GetMemoClientInfo(repoDir)
		if err != nil {
			return err
		}

		napi, closer, err := client.NewGenericNode(cctx.Context, addr, headers)
		if err != nil {
			return err
		}
		defer closer()

		nid, err := napi.RoleSelf(cctx.Context)
		if err != nil {
			return err
		}

		fmt.Println("pay info: ", nid.ID)

		spi, err := napi.GetAccPostIncome(cctx.Context, nid.ID)
		if err != nil {
			return err
		}

		fmt.Printf("pay info: pro %d, income value %s, penalty %s, signer: %d \n", nid.ID, types.FormatWei(spi.Value), types.FormatWei(spi.Penalty), spi.Sig.Signer)

		val, err := napi.GetBalance(cctx.Context, nid.ID)
		if err != nil {
			return err
		}

		fmt.Printf("pay info max: proID %d, expected income: %s \n", nid.ID, types.FormatWei(val))

		return nil
	},
}

var stateWithdrawCmd = &cli.Command{
	Name:  "withdraw",
	Usage: "withdraw balance",
	Action: func(cctx *cli.Context) error {
		repoDir := cctx.String(FlagNodeRepo)
		addr, headers, err := client.GetMemoClientInfo(repoDir)
		if err != nil {
			return err
		}

		napi, closer, err := client.NewGenericNode(cctx.Context, addr, headers)
		if err != nil {
			return err
		}
		defer closer()

		nid, err := napi.RoleSelf(cctx.Context)
		if err != nil {
			return err
		}

		spi, err := napi.GetAccPostIncome(cctx.Context, nid.ID)
		if err != nil {
			return err
		}

		fmt.Printf("pay info: pro %d, income value %s, penalty %s, signer: %d \n", nid.ID, types.FormatWei(spi.Value), types.FormatWei(spi.Penalty), spi.Sig.Signer)

		err = napi.Withdraw(cctx.Context, spi.Value, spi.Penalty)
		if err != nil {
			return err
		}

		return nil
	},
}
