package cmd

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"github.com/mgutz/ansi"
	"github.com/urfave/cli/v2"

	"github.com/memoio/go-mefs-v2/api/client"
	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/utils"
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

		fmt.Println(ansi.Color("----------- Information -----------", "green"))

		fmt.Println(time.Now())

		fmt.Println(ansi.Color("----------- Sync Information -----------", "green"))
		si, err := api.SyncGetInfo(cctx.Context)
		if err != nil {
			return err
		}

		sgi, err := api.StateGetInfo(cctx.Context)
		if err != nil {
			return err
		}

		nt := time.Now().Unix()
		st := build.BaseTime + int64(sgi.Slot*build.SlotDuration)
		lag := (nt - st) / build.SlotDuration

		fmt.Printf("Status: %t, Time: %s\n", si.Status && (si.SyncedHeight+5 > si.RemoteHeight) && (lag < 10), time.Unix(st, 0).Format(utils.SHOWTIME))
		fmt.Printf("Height Synced: %d, Remote: %d\n", si.SyncedHeight, si.RemoteHeight)

		fmt.Println(ansi.Color("----------- Role Information -----------", "green"))
		pri, err := api.RoleSelf(cctx.Context)
		if err != nil {
			return err
		}
		fmt.Println("ID: ", pri.ID)
		fmt.Println("Type: ", pri.Type.String())
		fmt.Printf("Wallet: %s \n", "0x"+hex.EncodeToString(pri.ChainVerifyKey))

		bi, err := api.SettleGetBalanceInfo(cctx.Context, pri.ID)
		if err != nil {
			return err
		}

		fmt.Printf("Balance: %s (on chain), %s (Erc20), %s (in fs)\n", types.FormatWei(bi.Value), types.FormatWei(bi.ErcValue), types.FormatWei(bi.FsValue))

		switch pri.Type {
		case pb.RoleInfo_Provider:
			size := uint64(0)
			price := big.NewInt(0)
			users := api.StateGetUsersAt(context.TODO(), pri.ID)
			for _, uid := range users {
				si, err := api.SettleGetStoreInfo(context.TODO(), uid, pri.ID)
				if err != nil {
					continue
				}
				size += si.Size
				price.Add(price, si.Price)
			}
			fmt.Printf("Data Stored: size %d byte (%s), price %d\n", size, types.FormatBytes(size), price)
		case pb.RoleInfo_User:
			size := uint64(0)
			price := big.NewInt(0)
			pros := api.StateGetProsAt(context.TODO(), pri.ID)
			for _, pid := range pros {
				si, err := api.SettleGetStoreInfo(context.TODO(), pri.ID, pid)
				if err != nil {
					continue
				}
				size += si.Size
				price.Add(price, si.Price)
			}
			fmt.Printf("Data Stored: size %d byte (%s), price %d\n", size, types.FormatBytes(size), price)
		}

		fmt.Println(ansi.Color("----------- Group Information -----------", "green"))
		gid := api.SettleGetGroupID(cctx.Context)
		gi, err := api.SettleGetGroupInfoAt(cctx.Context, gid)
		if err != nil {
			return err
		}

		fmt.Println("ID: ", gid)
		fmt.Println("Security Level: ", gi.Level)
		fmt.Println("Size: ", types.FormatBytes(gi.Size))
		fmt.Println("Price: ", gi.Price)
		fmt.Printf("Keepers: %d, Providers: %d, Users: %d\n", gi.KCount, gi.PCount, gi.UCount)

		fmt.Println(ansi.Color("----------- Pledge Information ----------", "green"))

		pi, err := api.SettleGetPledgeInfo(cctx.Context, pri.ID)
		if err != nil {
			return err
		}
		fmt.Printf("Pledge: %s, %s (total pledge), %s (total in pool)\n", types.FormatWei(pi.Value), types.FormatWei(pi.Total), types.FormatWei(pi.ErcTotal))

		if pri.Type == pb.RoleInfo_User {
			uapi, closer, err := client.NewUserNode(cctx.Context, addr, headers)
			if err != nil {
				return err
			}
			defer closer()
			fmt.Println(ansi.Color("----------- Lfs Information ----------", "green"))
			li, err := uapi.LfsGetInfo(cctx.Context)
			if err != nil {
				return err
			}

			fmt.Println("Status: ", li.Status)
			fmt.Println("Bucket: ", li.Bucket)
			fmt.Println("Used:", types.FormatBytes(li.Used))
		}

		return nil
	},
}
