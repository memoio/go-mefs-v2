package cmd

import (
	"path/filepath"
	"strconv"

	callconts "memoContract/callcontracts"

	"github.com/memoio/go-mefs-v2/app/minit"
	"github.com/memoio/go-mefs-v2/config"
	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/backend/keystore"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/utils"
	"github.com/memoio/go-mefs-v2/submodule/connect/settle"
	"github.com/memoio/go-mefs-v2/submodule/wallet"

	"github.com/ethereum/go-ethereum/common"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var TransferCmd = &cli.Command{
	Name:  "transfer",
	Usage: "transfer eth or erc20",
	Subcommands: []*cli.Command{
		transferEthCmd,
		transferErcCmd,
		addKeeperToGroupCmd,
	},
}

var addKeeperToGroupCmd = &cli.Command{
	Name:      "group",
	Usage:     "add keeper to group",
	ArgsUsage: "[group index]",
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return xerrors.Errorf("need group index")
		}

		gid, err := strconv.ParseUint(cctx.Args().First(), 10, 0)
		if err != nil {
			return xerrors.Errorf("parsing 'amount' argument: %w", err)
		}

		repoDir := cctx.String(FlagNodeRepo)

		configFile := filepath.Join(repoDir, "config.json")
		cfg, err := config.ReadFile(configFile)
		if err != nil {
			return xerrors.Errorf("failed to read config file at %q %w", configFile, err)
		}

		ar, err := address.NewFromString(cfg.Wallet.DefaultAddress)
		if err != nil {
			return xerrors.Errorf("failed to parse addr %s %w", cfg.Wallet.DefaultAddress, err)
		}

		ksp := filepath.Join(repoDir, "keystore")

		ks, err := keystore.NewKeyRepo(ksp)
		if err != nil {
			return err
		}

		pw, err := minit.GetPassWord()
		if err != nil {
			return err
		}

		lw := wallet.New(pw, ks)
		ki, err := lw.WalletExport(cctx.Context, ar)
		if err != nil {
			return err
		}

		cm, err := settle.NewContractMgr(cctx.Context, ki.SecretKey)
		if err != nil {
			return err
		}

		ri, err := cm.SettleGetRoleInfo(ar)
		if err != nil {
			return err
		}

		if ri.GroupID == 0 && gid > 0 {
			err = settle.AddKeeperToGroup(ri.GetID(), gid)
			if err != nil {
				return err
			}
		}

		return nil
	},
}

var transferEthCmd = &cli.Command{
	Name:      "eth",
	Usage:     "transfer eth",
	ArgsUsage: "[wallet address] [value]",
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 2 {
			return xerrors.Errorf("need two parameters")
		}

		repoDir := cctx.String(FlagNodeRepo)

		addr := cctx.Args().Get(0)

		toAdderss := common.HexToAddress(addr)
		if addr == "0x0" {
			configFile := filepath.Join(repoDir, "config.json")
			cfg, err := config.ReadFile(configFile)
			if err != nil {
				return xerrors.Errorf("failed to read config file at %q %w", configFile, err)
			}

			ar, err := address.NewFromString(cfg.Wallet.DefaultAddress)
			if err != nil {
				return xerrors.Errorf("failed to parse addr %s %w", cfg.Wallet.DefaultAddress, err)
			}

			toAdderss = common.BytesToAddress(utils.ToEthAddress(ar.Bytes()))
		}

		val, err := types.ParsetValue(cctx.Args().Get(1))
		if err != nil {
			return xerrors.Errorf("parsing 'amount' argument: %w", err)
		}

		return settle.TransferTo(toAdderss, val, callconts.AdminSk)
	},
}

var transferErcCmd = &cli.Command{
	Name:      "erc",
	Usage:     "transfer erc",
	ArgsUsage: "[wallet address] [value]",
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 2 {
			return xerrors.Errorf("need two parameters")
		}

		repoDir := cctx.String(FlagNodeRepo)

		addr := cctx.Args().Get(0)
		toAdderss := common.HexToAddress(addr)

		if addr == "0x0" {
			configFile := filepath.Join(repoDir, "config.json")
			cfg, err := config.ReadFile(configFile)
			if err != nil {
				return xerrors.Errorf("failed to read config file at %q %w", configFile, err)
			}
			ar, err := address.NewFromString(cfg.Wallet.DefaultAddress)
			if err != nil {
				return xerrors.Errorf("failed to parse addr %s %w", cfg.Wallet.DefaultAddress, err)
			}

			toAdderss = common.BytesToAddress(utils.ToEthAddress(ar.Bytes()))
		}

		val, err := types.ParsetValue(cctx.Args().Get(1))
		if err != nil {
			return xerrors.Errorf("parsing 'amount' argument: %w", err)
		}

		return settle.Erc20Transfer(toAdderss, val)
	},
}
