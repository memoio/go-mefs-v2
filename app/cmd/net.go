package cmd

import (
	"fmt"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/urfave/cli/v2"

	"github.com/memoio/go-mefs-v2/api/client"
	netutils "github.com/memoio/go-mefs-v2/lib/utils/net"
)

var NetCmd = &cli.Command{
	Name:  "net",
	Usage: "Interact with net",
	Subcommands: []*cli.Command{
		netInfoCmd,
		netConnectCmd,
		netPeersCmd,
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
		fmt.Println(pi.String())

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
			fmt.Printf("%s\n", peer.String())
		}
		return nil
	},
}

var netConnectCmd = &cli.Command{
	Name:      "connect",
	Usage:     "connet a peer",
	ArgsUsage: "[peerMultiaddr]",
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
	Name:  "findpeer",
	Usage: "find peers",
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

		fmt.Printf("got %s\n", info.String())

		return nil
	},
}
