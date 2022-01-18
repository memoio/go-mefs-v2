package cmd

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/memoio/go-mefs-v2/api/client"
	"github.com/memoio/go-mefs-v2/lib/pb"
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

		bi, err := api.GetBalanceInfo(cctx.Context, pri.ID)
		if err != nil {
			return err
		}

		fmt.Printf("Balance: %s (on chain), %s (Erc20), %s (in fs)\n", types.FormatWei(bi.Value), types.FormatWei(bi.ErcValue), types.FormatWei(bi.FsValue))

		switch pri.Type {
		case pb.RoleInfo_Provider:
			size := uint64(0)
			price := big.NewInt(0)
			users := api.GetUsersForPro(context.TODO(), pri.ID)
			for _, uid := range users {
				si, err := api.GetStoreInfo(context.TODO(), uid, pri.ID)
				if err != nil {
					continue
				}
				size += si.Size
				price.Add(price, si.Price)
			}
			fmt.Printf("Data Stored: size %d, price %d\n", size, price)
		case pb.RoleInfo_User:
			size := uint64(0)
			price := big.NewInt(0)
			pros := api.GetProsForUser(context.TODO(), pri.ID)
			for _, pid := range pros {
				si, err := api.GetStoreInfo(context.TODO(), pri.ID, pid)
				if err != nil {
					continue
				}
				size += si.Size
				price.Add(price, si.Price)
			}
			fmt.Printf("Data Stored: size %d, price %d\n", size, price)
		}

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

		fmt.Println("-----------")

		fmt.Println("Pledge Infomation")
		pi, err := api.GetPledgeInfo(cctx.Context, pri.ID)
		if err != nil {
			return err
		}
		fmt.Printf("Pledge: %s, %s (total pledge), %s (total in pool)\n", types.FormatWei(pi.Value), types.FormatWei(pi.Total), types.FormatWei(pi.ErcTotal))

		return nil
	},
}
