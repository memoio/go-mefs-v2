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
			pi, err := napi.GetPostIncome(cctx.Context, uid, nid.ID)
			if err != nil {
				continue
			}
			fmt.Printf("post income: proID %d, userID %d, value: %s, penalty: %s \n", nid.ID, uid, types.FormatWei(pi.Value), types.FormatWei(pi.Penalty))
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

		bal, err := napi.GetBalance(cctx.Context, nid.ID)
		if err != nil {
			return err
		}

		fmt.Printf("%d has balance %d \n", nid.ID, bal)

		fmt.Printf("withdraw info: pro %d, income value %s, penalty %s, signer: %d \n", nid.ID, types.FormatWei(spi.Value), types.FormatWei(spi.Penalty), spi.Sig.Signer)

		ksign := make([][]byte, spi.Sig.Len())
		for i := 0; i < spi.Sig.Len(); i++ {
			ksign[i] = spi.Sig.Data[65*i : 65*(i+1)]
		}

		err = napi.Withdraw(cctx.Context, spi.Value, spi.Penalty, ksign)
		if err != nil {
			fmt.Println("withdraw fail", err)
			return err
		}

		bal, err = napi.GetBalance(cctx.Context, nid.ID)
		if err != nil {
			return err
		}

		fmt.Printf("%d has balance %d \n", nid.ID, bal)

		return nil
	},
}
