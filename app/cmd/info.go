package cmd

import (
	"encoding/hex"
	"fmt"

	"github.com/memoio/go-mefs-v2/api/client"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/urfave/cli/v2"
)

var InfoCmd = &cli.Command{
	Name:  "info",
	Usage: "print information of this node",
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

		pri, err := api.RoleSelf(cctx.Context)
		if err != nil {
			return err
		}

		fmt.Println("Role Infomation")
		fmt.Println("ID: ", pri.ID)
		fmt.Println("Type: ", pri.Type.String())
		fmt.Printf("Wallet Address: %s \n", "0x"+hex.EncodeToString(pri.ChainVerifyKey))

		bi, err := api.GetBalance(cctx.Context, pri.ID)
		if err != nil {
			return err
		}

		fmt.Printf("Balance: %s (on chain), %s (Erc20), %s (in fs)\n", types.FormatWei(bi.Value), types.FormatWei(bi.ErcValue), types.FormatWei(bi.FsValue))

		fmt.Println("-----------")

		gid := api.GetGroupID(cctx.Context)
		gi, err := api.GetGroupInfoAt(cctx.Context, gid)
		if err != nil {
			return err
		}

		fmt.Println("Group Infomation")
		fmt.Println("ID: ", gid)
		fmt.Println("Security Level: ", gi.Level)
		fmt.Println("Size ", types.FormatBytes(gi.Size))
		fmt.Println("Price ", gi.Price)

		return nil
	},
}
