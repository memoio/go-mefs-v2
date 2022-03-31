package cmd

import (
	"path/filepath"
	"strconv"

	callconts "memoc/callcontracts"

	"github.com/memoio/go-mefs-v2/app/minit"
	"github.com/memoio/go-mefs-v2/config"
	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/backend/keystore"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/utils"
	"github.com/memoio/go-mefs-v2/submodule/connect/settle"
	"github.com/memoio/go-mefs-v2/submodule/wallet"
	"github.com/mitchellh/go-homedir"

	"github.com/ethereum/go-ethereum/common"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var TransferCmd = &cli.Command{
	Name:  "transfer",
	Usage: "transfer eth or memo",
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
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    pwKwd,
			Aliases: []string{"pwd"},
			Value:   "memoriae",
		},
		&cli.StringFlag{
			Name:  "sk",
			Usage: "secret key of admin",
			Value: "",
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return xerrors.Errorf("need group index")
		}

		gid, err := strconv.ParseUint(cctx.Args().First(), 10, 0)
		if err != nil {
			return xerrors.Errorf("parsing 'amount' argument: %w", err)
		}

		repoDir := cctx.String(FlagNodeRepo)
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

		ksp := filepath.Join(repoDir, "keystore")

		ks, err := keystore.NewKeyRepo(ksp)
		if err != nil {
			return err
		}
		pw := cctx.String(pwKwd)
		if pw == "" {
			pw, err = minit.GetPassWord()
			if err != nil {
				return err
			}
		}

		lw := wallet.New(pw, ks)
		ki, err := lw.WalletExport(cctx.Context, ar, pw)
		if err != nil {
			return err
		}

		cm, err := settle.NewContractMgr(cctx.Context, cfg.Contract.EndPoint, cfg.Contract.RoleContract, ki.SecretKey)
		if err != nil {
			return err
		}

		ri, err := cm.SettleGetRoleInfo(ar)
		if err != nil {
			return err
		}

		if ri.RoleID == 0 {
			return xerrors.Errorf("role is not registered")
		}

		if ri.Type != pb.RoleInfo_Keeper {
			return xerrors.Errorf("role is not keeper")
		}

		if ri.GroupID == 0 && gid > 0 {
			sk := cctx.String("sk")
			err = settle.AddKeeperToGroup(cfg.Contract.EndPoint, cfg.Contract.RoleContract, sk, ri.RoleID, gid)
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
	ArgsUsage: "[wallet address (0x...)] [amount (Token / Gwei / Wei)]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "endPoint",
			Value: callconts.EndPoint,
		},
		&cli.StringFlag{
			Name:  "sk",
			Usage: "secret key of admin",
			Value: "",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 2 {
			return xerrors.Errorf("need two parameters")
		}

		repoDir := cctx.String(FlagNodeRepo)
		repoDir, err := homedir.Expand(repoDir)
		if err != nil {
			return err
		}

		addr := cctx.Args().Get(0)
		toAdderss := common.HexToAddress(addr)

		val, err := types.ParsetEthValue(cctx.Args().Get(1))
		if err != nil {
			return xerrors.Errorf("parsing 'amount' argument: %w", err)
		}

		ep := cctx.String("endPoint")
		sk := cctx.String("sk")

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
			ep = cfg.Contract.EndPoint
		}

		return settle.TransferTo(ep, toAdderss, val, sk)
	},
}

var transferErcCmd = &cli.Command{
	Name:      "memo",
	Usage:     "transfer memo",
	ArgsUsage: "[wallet address (0x...)] [amount (Memo / NanoMemo / AttoMemo)]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "endPoint",
			Value: callconts.EndPoint,
		},
		&cli.StringFlag{
			Name:  "roleContract",
			Usage: "address role contract",
			Value: callconts.RoleAddr.String(),
		},
		&cli.StringFlag{
			Name:  "sk",
			Usage: "secret key of admin",
			Value: "",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 2 {
			return xerrors.Errorf("need two parameters: address and value")
		}

		repoDir := cctx.String(FlagNodeRepo)
		repoDir, err := homedir.Expand(repoDir)
		if err != nil {
			return err
		}

		addr := cctx.Args().Get(0)
		toAdderss := common.HexToAddress(addr)

		val, err := types.ParsetValue(cctx.Args().Get(1))
		if err != nil {
			return xerrors.Errorf("parsing 'amount' argument: %w", err)
		}

		ep := cctx.String("endPoint")
		rAddr := common.HexToAddress(cctx.String("roleContract"))
		sk := cctx.String("sk")

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

			rAddr = common.HexToAddress(cfg.Contract.RoleContract)
			ep = cfg.Contract.EndPoint
		}

		rtAddr, err := settle.GetRoleTokenAddr(ep, rAddr, toAdderss)
		if err != nil {
			return err
		}

		tAddr, err := settle.GetTokenAddr(ep, rtAddr, toAdderss, 0)
		if err != nil {
			return err
		}
		return settle.TransferMemoTo(ep, sk, tAddr, toAdderss, val)
	},
}
