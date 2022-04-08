package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/api/client"
	"github.com/memoio/go-mefs-v2/lib/tx"
	netutils "github.com/memoio/go-mefs-v2/lib/utils/net"
)

var netCmd = &cli.Command{
	Name:  "net",
	Usage: "Interact with net",
	Subcommands: []*cli.Command{
		netInfoCmd,
		netConnectCmd,
		netPeersCmd,
		findpeerCmd,
		declareCmd,
	},
}

var netInfoCmd = &cli.Command{
	Name:  "info",
	Usage: "get net info",
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

		pi, err := napi.NetAddrInfo(cctx.Context)
		if err != nil {
			return err
		}

		ni, err := napi.NetAutoNatStatus(cctx.Context)
		if err != nil {
			return err
		}

		addrs := make([]string, 0, len(pi.Addrs))
		for _, maddr := range pi.Addrs {
			saddr := maddr.String()
			if strings.Contains(saddr, "/127.0.0.1/") {
				continue
			}

			if strings.Contains(saddr, "/::1/") {
				continue
			}

			addrs = append(addrs, saddr)
		}

		switch ni.Reachability {
		case network.ReachabilityPublic:
			fmt.Printf("Network ID %s, IP %s, Type: %s, %s \n", pi.ID, addrs, ni.Reachability, ni.PublicAddr)
		default:
			fmt.Printf("Network ID %s, IP %s, Type: %s \n", pi.ID, addrs, ni.Reachability)
		}

		return nil
	},
}

var netPeersCmd = &cli.Command{
	Name:  "peers",
	Usage: "print peers",
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

		info, err := napi.NetPeers(cctx.Context)
		if err != nil {
			return err
		}

		for _, peer := range info {
			addrs := make([]string, 0, len(peer.Addrs))
			for _, maddr := range peer.Addrs {
				saddr := maddr.String()
				if strings.Contains(saddr, "/127.0.0.1/") {
					continue
				}

				if strings.Contains(saddr, "/::1/") {
					continue
				}

				addrs = append(addrs, saddr)
			}

			fmt.Printf("%s %s\n", peer.ID, addrs)
		}
		return nil
	},
}

var netConnectCmd = &cli.Command{
	Name:      "connect",
	Usage:     "connet a peer",
	ArgsUsage: "[peer multiaddr (/ip4/1.2.3.4/tcp/5678/p2p/12D...)]",
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

		pis, err := netutils.ParseAddresses(cctx.Args().Slice())
		if err != nil {
			return err
		}

		for _, pa := range pis {
			err := napi.NetConnect(cctx.Context, pa)
			if err != nil {
				return err
			}
			fmt.Printf("connect to: %s\n", pa.String())
		}

		return nil
	},
}

var findpeerCmd = &cli.Command{
	Name:      "findpeer",
	Usage:     "find peers",
	ArgsUsage: "[peerID (12D...)]",
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

		p := cctx.Args().First()
		pid, err := peer.Decode(p)
		if err != nil {
			return err
		}
		info, err := napi.NetFindPeer(cctx.Context, pid)
		if err != nil {
			return err
		}

		epi, err := napi.NetPeerInfo(cctx.Context, pid)
		if err != nil {
			return err
		}

		fmt.Println(info.String(), epi.Agent, epi.Protocols)

		return nil
	},
}

var declareCmd = &cli.Command{
	Name:      "declare",
	Usage:     "declare public network address",
	ArgsUsage: "[net address (/ip4/1.2.3.4/tcp/5678)]",
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 1 {
			return xerrors.Errorf("require one parameter")
		}

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

		pai, err := napi.NetAddrInfo(cctx.Context)
		if err != nil {
			return err
		}

		rid := napi.SettleGetRoleID(cctx.Context)

		maddr, err := ma.NewMultiaddr(cctx.Args().First())
		if err != nil {
			return err
		}

		pai.Addrs = []ma.Multiaddr{maddr}

		data, err := pai.MarshalJSON()
		if err != nil {
			return err
		}
		msg := &tx.Message{
			Version: 0,
			From:    rid,
			To:      rid,
			Method:  tx.UpdateNet,
			Params:  data,
		}

		ctx := cctx.Context
		mid, err := napi.PushMessage(ctx, msg)
		if err != nil {
			return xerrors.Errorf("push fail %s", err)
		}

		fmt.Println("update net addr to: ", pai.String())
		fmt.Println("waiting tx message done: ", mid)

		ctx, cancle := context.WithTimeout(ctx, 6*time.Minute)
		defer cancle()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			st, err := napi.SyncGetTxMsgStatus(ctx, mid)
			if err != nil {
				time.Sleep(10 * time.Second)
				continue
			}

			if st.Status.Err == 0 {
				fmt.Println("tx message done success: ", mid, msg.From, msg.To, msg.Method, st.BlockID, st.Height)
				fmt.Println("updated net addr to: ", pai.String())
			} else {
				fmt.Println("tx message done fail: ", mid, msg.From, msg.To, msg.Method, st.BlockID, st.Height, st.Status)
			}
			break
		}

		return nil
	},
}
