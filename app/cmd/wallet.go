package cmd

import (
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/memoio/go-mefs-v2/api/client"
	"github.com/memoio/go-mefs-v2/lib/types"
)

var WalletCmd = &cli.Command{
	Name:  "wallet",
	Usage: "Interact with wallet",
	Subcommands: []*cli.Command{
		walletnewCmd,
		walletListCmd,
	},
}

var walletListCmd = &cli.Command{
	Name:  "list",
	Usage: "list all addrs",
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

		addrs, err := api.WalletList(cctx.Context)
		if err != nil {
			return err
		}

		for _, as := range addrs {
			fmt.Println(as)
		}
		return nil
	},
}

var walletnewCmd = &cli.Command{
	Name:  "new",
	Usage: "create a new wallet address",
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

		waddr, err := api.WalletNew(cctx.Context, types.Secp256k1)
		if err != nil {
			return err
		}
		fmt.Println(waddr)

		return nil
	},
}
