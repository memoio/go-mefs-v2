package cmd

import (
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/api/client"
	"github.com/memoio/go-mefs-v2/app/minit"
	"github.com/memoio/go-mefs-v2/config"
	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/backend/keystore"
	"github.com/memoio/go-mefs-v2/lib/repo"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/utils"
)

var walletCmd = &cli.Command{
	Name:  "wallet",
	Usage: "Interact with wallet",
	Subcommands: []*cli.Command{
		walletnewCmd,
		walletListCmd,
		walletDefaultCmd,
		walletExportCmd,
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

		// get client info
		addr, headers, err := client.GetMemoClientInfo(repoDir)
		// get wallet list using api
		if err == nil {
			// generate client node
			api, closer, err := client.NewGenericNode(cctx.Context, addr, headers)
			if err != nil {
				return err
			}
			defer closer()

			// get wallet list
			addrs, err := api.WalletList(cctx.Context)
			if err != nil {
				return err
			}

			fmt.Println("Wallet list:")
			for _, as := range addrs {
				if as.Len() == 20 {
					toAddress := common.BytesToAddress(as.Bytes())
					fmt.Println(toAddress)
				}
			}
		} else {
			// if node is not running, get wallet list from repo directly

			// read wallet list
			repoDir := cctx.String(FlagNodeRepo)
			// generate a repo from repoDir
			rep, err := repo.NewFSRepo(repoDir, nil, true)
			if err != nil {
				return err
			}
			defer rep.Close()

			// get keystore from repo
			ks := rep.KeyStore()
			// get all wallet address in keystore
			walletlist, err := ks.List()
			if err != nil {
				return err
			}

			// address.Address
			wallets := make([]address.Address, 0, len(walletlist))

			// transfer wallet to address type
			for _, s := range walletlist {
				if strings.HasPrefix(s, address.AddrPrefix) {
					addr, err := address.NewFromString(s)
					if err != nil {
						continue
					}

					wallets = append(wallets, addr)
				}
			}

			// sort
			sort.Slice(wallets, func(i, j int) bool {
				return wallets[i].String() < wallets[j].String()
			})

			// show wallets
			fmt.Println()
			fmt.Println("Wallet list:")
			for _, as := range wallets {
				if as.Len() == 20 {
					toAddress := common.BytesToAddress(as.Bytes())
					fmt.Println(toAddress)
				}
			}
			fmt.Println()
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

var walletExportCmd = &cli.Command{
	Name:      "export",
	Usage:     "export wallet secret key",
	ArgsUsage: "[wallet address (0x...)]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    pwKwd,
			Aliases: []string{"pwd"},
			Value:   "memoriae",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 1 {
			return xerrors.Errorf("need one parameter")
		}

		// trans address format
		toAdderss := common.HexToAddress(cctx.Args().Get(0))
		maddr, err := address.NewAddress(toAdderss.Bytes())
		if err != nil {
			return err
		}

		// get password
		pw := cctx.String(pwKwd)
		if pw == "" {
			pw, err = minit.GetPassWord()
			if err != nil {
				return err
			}
		}

		repoDir := cctx.String(FlagNodeRepo)
		addr, headers, err := client.GetMemoClientInfo(repoDir)
		if err == nil {
			api, closer, err := client.NewGenericNode(cctx.Context, addr, headers)
			if err != nil {
				return err
			}
			defer closer()

			ki, err := api.WalletExport(cctx.Context, maddr, pw)
			if err != nil {
				return err
			}
			fmt.Println("secret key: ", hex.EncodeToString(ki.SecretKey))
		} else {
			repoDir, err = homedir.Expand(repoDir)
			if err != nil {
				return err
			}
			kfile := filepath.Join(repoDir, "keystore", maddr.String())
			sk, err := keystore.LoadKeyFile(pw, kfile)
			if err != nil {
				return err
			}
			fmt.Println("secret key: ", sk)
		}

		return nil
	},
}
