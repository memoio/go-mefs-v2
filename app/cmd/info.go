package cmd

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/mgutz/ansi"
	"github.com/urfave/cli/v2"

	"github.com/memoio/go-mefs-v2/api/client"
	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/utils"
)

var infoCmd = &cli.Command{
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

		fmt.Println(ansi.Color("----------- Information -----------", "green"))

		ver, err := api.Version(cctx.Context)
		if err != nil {
			return err
		}

		fmt.Println(time.Now().Format(utils.SHOWTIME))
		fmt.Println(ver)

		fmt.Println(ansi.Color("----------- Network Information -----------", "green"))
		npi, err := api.NetAddrInfo(cctx.Context)
		if err != nil {
			return err
		}

		nni, err := api.NetAutoNatStatus(cctx.Context)
		if err != nil {
			return err
		}

		addrs := make([]string, 0, len(npi.Addrs))
		for _, maddr := range npi.Addrs {
			saddr := maddr.String()
			if strings.Contains(saddr, "/127.0.0.1/") {
				continue
			}

			if strings.Contains(saddr, "/::1/") {
				continue
			}

			addrs = append(addrs, saddr)
		}

		fmt.Println("ID: ", npi.ID)
		fmt.Println("IP: ", addrs)
		fmt.Printf("Type: %s %s\n", nni.Reachability, nni.PublicAddr)

		sni, err := api.StateGetNetInfo(cctx.Context, pri.RoleID)
		if err == nil {
			fmt.Println("Declared Address: ", sni.String())
		}

		fmt.Println(ansi.Color("----------- Sync Information -----------", "green"))
		si, err := api.SyncGetInfo(cctx.Context)
		if err != nil {
			return err
		}

		sgi, err := api.StateGetInfo(cctx.Context)
		if err != nil {
			return err
		}

		ce, err := api.StateGetChalEpochInfo(cctx.Context)
		if err != nil {
			return err
		}

		nt := time.Now().Unix()
		st := build.BaseTime + int64(sgi.Slot*build.SlotDuration)
		lag := (nt - st) / build.SlotDuration

		fmt.Printf("Status: %t, Slot: %d, Time: %s\n", si.Status && (si.SyncedHeight+5 > si.RemoteHeight) && (lag < 10), sgi.Slot, time.Unix(st, 0).Format(utils.SHOWTIME))
		fmt.Printf("Height Synced: %d, Remote: %d\n", si.SyncedHeight, si.RemoteHeight)
		fmt.Println("Challenge Epoch:", ce.Epoch, time.Unix(build.BaseTime+int64(ce.Slot*build.SlotDuration), 0).Format(utils.SHOWTIME))

		fmt.Println(ansi.Color("----------- Role Information -----------", "green"))

		fmt.Println("ID: ", pri.RoleID)
		fmt.Println("Type: ", pri.Type.String())
		fmt.Println("Wallet: ", common.BytesToAddress(pri.ChainVerifyKey))

		bi, err := api.SettleGetBalanceInfo(cctx.Context, pri.RoleID)
		if err != nil {
			return err
		}

		fmt.Printf("Balance: %s (tx fee), %s (Erc20), %s (in fs)\n", types.FormatEth(bi.Value), types.FormatMemo(bi.ErcValue), types.FormatMemo(bi.FsValue))

		switch pri.Type {
		case pb.RoleInfo_Provider:
			size := uint64(0)
			price := big.NewInt(0)
			users := api.StateGetUsersAt(context.TODO(), pri.RoleID)
			for _, uid := range users {
				si, err := api.SettleGetStoreInfo(context.TODO(), uid, pri.RoleID)
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
			pros := api.StateGetProsAt(context.TODO(), pri.RoleID)
			for _, pid := range pros {
				si, err := api.SettleGetStoreInfo(context.TODO(), pri.RoleID, pid)
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

		fmt.Println("EndPoint: ", gi.EndPoint)
		fmt.Println("Contract Address: ", gi.RoleAddr)
		fmt.Println("Fs Address: ", gi.FsAddr)
		fmt.Println("ID: ", gid)
		fmt.Println("Security Level: ", gi.Level)
		fmt.Println("Size: ", types.FormatBytes(gi.Size))
		fmt.Println("Price: ", gi.Price)
		fmt.Printf("Keepers: %d, Providers: %d, Users: %d\n", gi.KCount, gi.PCount, gi.UCount)

		fmt.Println(ansi.Color("----------- Pledge Information ----------", "green"))

		pi, err := api.SettleGetPledgeInfo(cctx.Context, pri.RoleID)
		if err != nil {
			return err
		}
		fmt.Printf("Pledge: %s, %s (total pledge), %s (total in pool)\n", types.FormatMemo(pi.Value), types.FormatMemo(pi.Total), types.FormatMemo(pi.ErcTotal))

		switch pri.Type {
		case pb.RoleInfo_User:
			uapi, closer, err := client.NewUserNode(cctx.Context, addr, headers)
			if err != nil {
				return err
			}
			defer closer()
			fmt.Println(ansi.Color("----------- Lfs Information ----------", "green"))
			li, err := uapi.LfsGetInfo(cctx.Context, true)
			if err != nil {
				return err
			}

			pi, err := uapi.OrderGetPayInfoAt(cctx.Context, 0)
			if err != nil {
				return err
			}

			if li.Status {
				fmt.Println("Status: writable")
			} else {
				fmt.Println("Status: read only")
			}

			fmt.Println("Buckets: ", li.Bucket)
			fmt.Println("Used:", types.FormatBytes(li.Used))
			fmt.Println("Raw Size:", types.FormatBytes(pi.Size))
			fmt.Println("Confirmed Size:", types.FormatBytes(pi.ConfirmSize))
			fmt.Println("OnChain Size:", types.FormatBytes(pi.OnChainSize))
			fmt.Println("Need Pay:", types.FormatMemo(pi.NeedPay))
			fmt.Println("Paid:", types.FormatMemo(pi.Paid))
		case pb.RoleInfo_Provider:
			papi, closer, err := client.NewProviderNode(cctx.Context, addr, headers)
			if err != nil {
				return err
			}
			defer closer()

			pi, err := papi.OrderGetPayInfoAt(cctx.Context, 0)
			if err != nil {
				return err
			}

			fmt.Println(ansi.Color("----------- Store Information ----------", "green"))
			fmt.Println("Service Ready: ", papi.Ready(cctx.Context))
			fmt.Println("Received Size: ", types.FormatBytes(pi.Size))
			fmt.Println("Confirmed Size:", types.FormatBytes(pi.ConfirmSize))
			fmt.Println("OnChain Size: ", types.FormatBytes(pi.OnChainSize))
		}

		fmt.Println(ansi.Color("----------- Local Information -----------", "green"))
		lm, err := api.LocalStoreGetMeta(cctx.Context)
		if err != nil {
			return err
		}

		fmt.Printf("Meta Usage: path %s, used %s, free %s\n", lm.Path, types.FormatBytes(lm.Used), types.FormatBytes(lm.Free))

		dm, err := api.LocalStoreGetData(cctx.Context)
		if err != nil {
			return err
		}

		fmt.Printf("Data Usage: path %s, used %s, free %s\n", dm.Path, types.FormatBytes(dm.Used), types.FormatBytes(dm.Free))

		return nil
	},
}
