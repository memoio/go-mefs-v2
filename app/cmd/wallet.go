package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/api/client"
	"github.com/memoio/go-mefs-v2/config"
	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/utils"
)

var WalletCmd = &cli.Command{
	Name:  "wallet",
	Usage: "Interact with wallet",
	Subcommands: []*cli.Command{
		walletnewCmd,
		walletListCmd,
		walletDefaultCmd,
	},
}

var walletDefaultCmd = &cli.Command{
	Name:  "default",
	Usage: "print default wallet address",
	Action: func(cctx *cli.Context) error {
		repoDir := cctx.String(FlagNodeRepo)

		addr, headers, err := client.GetMemoClientInfo(repoDir)
		if err == nil {
			api, closer, err := client.NewGenericNode(cctx.Context, addr, headers)
			if err != nil {
				return err
			}
			defer closer()

			res, err := api.ConfigGet(cctx.Context, "wallet.address")
			if err != nil {
				return err
			}

			ar, err := address.NewFromString(res.(string))
			if err != nil {
				return xerrors.Errorf("failed to parse addr %s %w", res.(string), err)
			}

			toAddress := common.BytesToAddress(utils.ToEthAddress(ar.Bytes()))
			fmt.Println("wallet: ", toAddress)
			return nil
		}

		repoDir, err = homedir.Expand(repoDir)
		if err != nil {
			return err
		}
		configFile := filepath.Join(repoDir, "config.json")
		cfg, err := config.ReadFile(configFile)
		if err != nil {
			return xerrors.Errorf("failed to read config file at %q %w", configFile, err)
		}
		ar, err := address.NewFromString(cfg.Wallet.DefaultAddress)
		if err != nil {
			return xerrors.Errorf("failed to parse addr %s %w", cfg.Wallet.DefaultAddress, err)
		}

		toAddress := common.BytesToAddress(utils.ToEthAddress(ar.Bytes()))
		fmt.Println("wallet: ", toAddress)
		return nil
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
