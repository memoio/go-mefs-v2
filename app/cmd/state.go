package cmd

import (
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/memoio/go-mefs-v2/api/client"
)

var StateCmd = &cli.Command{
	Name:  "state",
	Usage: "Interact with state manager",
	Subcommands: []*cli.Command{
		statePostIncomeCmd,
	},
}

var statePostIncomeCmd = &cli.Command{
	Name:  "post",
	Usage: "list post of providers",
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
			fmt.Println("post income: ", nid.ID, uid, pi.Value)
		}

		return nil
	},
}
