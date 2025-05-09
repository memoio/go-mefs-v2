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
	Subcommands: []*cli.Command{
		selfInfoCmd,
	},
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "update",
			Value: false,
			Usage: "Update role info in memory by read it from db.",
		},
		&cli.BoolFlag{
			Name:    "all",
			Aliases: []string{"a", "v"},
			Value:   false, //默认值为空，要么手动输入，要么从本地keystore读取（需指定路径和密码）
			Usage:   "show all info",
		},
	},
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

		if cctx.Bool("update") {
			pri, err = api.RoleGet(cctx.Context, pri.RoleID, true)
			if err != nil {
				return err
			}
		}

		showAll := cctx.Bool("all")

		fmt.Println(ansi.Color("----------- Version Information -----------", "green"))

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

		fmt.Println("Network ID: ", npi.ID)
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

		fmt.Println(ansi.Color("----------- Role and Account Information -----------", "green"))

		fmt.Println("Role ID: ", pri.RoleID)
		fmt.Println("Type: ", pri.Type.String())

		switch string(pri.Desc) {
		case "cloud":
			fmt.Println("Location: Cloud")
		default:
			fmt.Println("Location: Personal")
		}
		fmt.Println("Wallet: ", common.BytesToAddress(pri.ChainVerifyKey))
		ri, err := api.SettleGetRoleInfoAt(cctx.Context, pri.RoleID)
		if err != nil {
			return err
		}

		fmt.Println("Owner: ", ri.Owner)
		fmt.Println()

		// GET FS BALANCE INFO
		bi, err := api.SettleGetBalanceInfo(cctx.Context, pri.RoleID)
		if err != nil {
			return err
		}
		// GET PLEDGE BALANCE INFO
		pi, err := api.SettleGetPledgeInfo(cctx.Context, pri.RoleID)
		if err != nil {
			return err
		}

		// display balance
		fmt.Printf("Memo Balance: %s\n", types.FormatMemo(bi.ErcValue))
		if showAll {
			fmt.Printf("cMemo Balance: %s\n", types.FormatEth(bi.Value))
		}

		if showAll {
			switch pri.Type {
			case pb.RoleInfo_Provider:
				// totalIncome from state db
				var totalIncome = new(big.Int)
				spi, err := api.StateGetAccPostIncome(cctx.Context, pri.RoleID)
				if err == nil {
					// got existed record from state db, and set totalIncome to it
					totalIncome.Set(spi.Value)
				}

				// get haspaid from settle info
				settleInfo, err := api.SettleGetSettleInfo(cctx.Context, pri.RoleID)
				if err != nil {
					return err
				}

				// calc provider income with total income and haspaid
				totalIncome.Sub(totalIncome, settleInfo.HasPaid)
				fmt.Printf("Storage Balance: %s, Income: %s\n", types.FormatMemo(bi.FsValue), types.FormatMemo(totalIncome))
			case pb.RoleInfo_User:
				fmt.Printf("Storage Balance: %s, Voucher: %s\n", types.FormatMemo(bi.FsValue), types.FormatMemo(bi.LockValue))
			default:
				fmt.Printf("Storage Balance: %s\n", types.FormatMemo(bi.FsValue))
			}
		} else {
			fmt.Printf("Storage Balance: %s\n", types.FormatMemo(bi.FsValue))
		}

		if pi.PledgeTime != nil && pi.PledgeTime.Cmp(big.NewInt(0)) > 0 {
			curReward := new(big.Int).Sub(pi.Value, pi.Last)
			fmt.Printf("Current Pledge: %s, Reward: %s\n", types.FormatMemo(pi.Last), types.FormatMemo(curReward))
			fmt.Printf("Pledge Time: %s\n", time.Unix(pi.PledgeTime.Int64(), 0).Format(utils.SHOWTIME))

			if showAll {
				localReward := new(big.Int).Sub(curReward, pi.CurReward)
				localReward = localReward.Add(localReward, pi.LocalReward)
				fmt.Printf("Historical Pledge: %s, Reward: %s\n", types.FormatMemo(pi.LocalPledge), types.FormatMemo(localReward))
				pledgeWithdraw := new(big.Int).Sub(pi.LocalPledge, pi.Last)
				pledgeRewardWithdraw := new(big.Int).Sub(pi.LocalReward, pi.CurReward)
				fmt.Printf("Withdraw Pledge: %s, Reward: %s\n", types.FormatMemo(pledgeWithdraw), types.FormatMemo(pledgeRewardWithdraw))

				fmt.Printf("Pool Pledge: %s, Reward: %s\n", types.FormatMemo(pi.Total), types.FormatMemo(new(big.Int).Sub(pi.ErcTotal, pi.Total)))
			}
		} else {
			// pledgeBal = pledge - (pledge+rs-last) = pledge-pledge-rs+last=last-rs
			// if (pledgeBal>=0)，rewardBal=rs；
			// else，pledgeBal=0，rewardBal=last

			curReward := new(big.Int).Sub(pi.Value, pi.Last)
			pledgeBal := new(big.Int).Sub(pi.Last, pi.LocalReward)
			rewardBal := new(big.Int).Set(pi.LocalReward)
			if pi.Last.Cmp(pi.LocalReward) > 0 {
				rewardBal.Add(rewardBal, curReward)
			} else {
				pledgeBal.SetInt64(0)
				rewardBal.Add(pi.Last, curReward)
			}

			fmt.Printf("Current Pledge: %s, Reward: %s\n", types.FormatMemo(pledgeBal), types.FormatMemo(rewardBal))

			if showAll {
				// total withdraw = total pledge + total reward - last
				totalWithdraw := new(big.Int).Add(pi.LocalPledge, pi.LocalReward)
				totalWithdraw.Sub(totalWithdraw, pi.Last)
				fmt.Printf("Historical Pledge: %s, Reward: %s, Withdraw: %s\n", types.FormatMemo(pi.LocalPledge), types.FormatMemo(pi.LocalReward), types.FormatMemo(totalWithdraw))

				fmt.Printf("Pool Pledge: %s, Reward: %s\n", types.FormatMemo(pi.Total), types.FormatMemo(new(big.Int).Sub(pi.ErcTotal, pi.Total)))
			}
		}

		switch pri.Type {
		case pb.RoleInfo_Provider:
			size := uint64(0)
			price := big.NewInt(0)
			users, err := api.StateGetUsersAt(context.TODO(), pri.RoleID)
			if err == nil {
				for _, uid := range users {
					si, err := api.SettleGetStoreInfo(context.TODO(), uid, pri.RoleID)
					if err != nil {
						continue
					}
					size += si.Size
					price.Add(price, si.Price)
				}
			}

			fmt.Println()
			if showAll {
				fmt.Printf("Storage Size: %d byte (%s), Price: %d (AttoMemo/Second)\n", size, types.FormatBytes(size), price)
			} else {
				fmt.Printf("Storage Size: %d byte (%s)\n", size, types.FormatBytes(size))
			}

			// calc size pledge for provider
			gi, err := api.SettleGetGroupInfoAt(cctx.Context, pri.GroupID)
			if err != nil {
				return err
			}
			// sizePledge = pPB * size
			sizePledge := new(big.Int).SetUint64(size)
			sizePledge.Mul(sizePledge, gi.PpB)
			fmt.Printf("Storage Deposit: %s\n", types.FormatMemo(sizePledge))
		case pb.RoleInfo_User:
			size := uint64(0)
			price := big.NewInt(0)
			pros, err := api.StateGetProsAt(context.TODO(), pri.RoleID)
			if err == nil {
				for _, pid := range pros {
					si, err := api.SettleGetStoreInfo(context.TODO(), pri.RoleID, pid)
					if err != nil {
						continue
					}
					size += si.Size
					price.Add(price, si.Price)
				}
			}

			fmt.Println()
			if showAll {
				fmt.Printf("Storage Size: %d byte (%s), Price %d (AttoMemo/Second)\n", size, types.FormatBytes(size), price)
			} else {
				fmt.Printf("Storage Size: %d byte (%s)\n", size, types.FormatBytes(size))
			}
		case pb.RoleInfo_Keeper:
			// calc size pledge for keeper
			gi, err := api.SettleGetGroupInfoAt(cctx.Context, pri.GroupID)
			if err != nil {
				return err
			}
			// sizePledge = pPB * size
			sizePledge := new(big.Int).SetUint64(gi.Size)
			sizePledge.Mul(sizePledge, gi.KpB)
			fmt.Println()
			fmt.Printf("Storage Deposit: %s\n", types.FormatMemo(sizePledge))
		}

		if showAll {
			fmt.Println(ansi.Color("----------- Group Information -----------", "green"))
			gid := api.SettleGetGroupID(cctx.Context)
			gi, err := api.SettleGetGroupInfoAt(cctx.Context, gid)
			if err != nil {
				return err
			}

			fmt.Println("EndPoint: ", gi.EndPoint)
			fmt.Println("Contract Address: ", gi.BaseAddr)
			fmt.Println("Group ID: ", gid)
			fmt.Println("Security Level: ", gi.Level)
			fmt.Println("Size: ", types.FormatBytes(gi.Size))
			fmt.Printf("Price: %d (AttoMemo/Second)\n", gi.Price)

			pros, err := api.RoleGetRelated(cctx.Context, pb.RoleInfo_Provider)
			if err != nil {
				return err
			}

			fmt.Printf("Keepers: %d, Providers: %d\n", gi.KCount, len(pros))
		}

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

			pi.NeedPay.Sub(pi.NeedPay, pi.Paid)
			fsbal := new(big.Int).Add(bi.LockValue, bi.FsValue) // fs中可用余额
			fsbal.Add(fsbal, bi.ErcValue)                       // 加上账户可充值到fs中的余额
			if pi.NeedPay.Cmp(fsbal) > 0 {
				fmt.Println(ansi.Color("Memo balance is not enough to pay and at least "+types.FormatMemo(fsbal.Sub(pi.NeedPay, fsbal))+" is still required", "red"))
			}
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
		lm, err := api.LocalStoreGetStat(cctx.Context, "kv")
		if err != nil {
			return err
		}

		fmt.Printf("Meta Usage: path %s, used %s, free %s\n", lm.Path, types.FormatBytes(lm.Used), types.FormatBytes(lm.Free))

		dm, err := api.LocalStoreGetStat(cctx.Context, "data")
		if err != nil {
			return err
		}

		fmt.Printf("Data Usage: path %s, used %s, free %s\n", dm.Path, types.FormatBytes(dm.Used), types.FormatBytes(dm.Free))

		return nil
	},
}

var selfInfoCmd = &cli.Command{
	Name:  "self",
	Usage: "print node id",
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

		fmt.Println("ID: ", pri.RoleID, pri.Type, pri.GroupID)

		return nil
	},
}
