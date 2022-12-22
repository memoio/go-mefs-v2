package cmd

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"path/filepath"
	"strconv"

	"github.com/memoio/go-mefs-v2/app/minit"
	"github.com/memoio/go-mefs-v2/config"
	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/backend/keystore"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/utils"
	"github.com/memoio/go-mefs-v2/submodule/connect/settle"
	v2 "github.com/memoio/go-mefs-v2/submodule/connect/v2"
	"github.com/memoio/go-mefs-v2/submodule/wallet"
	"github.com/mitchellh/go-homedir"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
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
	Usage:     "transfer eth, payer's sk can be provided, if not, use wallet address as payer",
	ArgsUsage: "[receiver address (0x...)] [amount (Token / Gwei / Wei)]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "endPoint",
			Aliases: []string{"ep"},
			Usage:   "endpoint of chain, if not given, local config will be used.",
			Value:   "",
		},
		&cli.StringFlag{
			Name:    "secretKey",
			Aliases: []string{"sk"},
			Usage:   "secret key of admin, if not given, local config will be used.",
			Value:   "",
		},
	},
	Action: func(cctx *cli.Context) error {
		fmt.Println("Transfering eth..")

		if cctx.Args().Len() != 2 {
			return xerrors.Errorf("need two parameters")
		}

		// get toAddress from param
		addr := cctx.Args().Get(0)
		toAdderss := common.HexToAddress(addr)

		// get value from param
		val, err := types.ParsetEthValue(cctx.Args().Get(1))
		if err != nil {
			return xerrors.Errorf("parsing 'amount' argument: %w", err)
		}
		if val.Cmp(big.NewInt(0)) == 0 {
			return xerrors.Errorf("amount can't be zero.")
		}

		ep := cctx.String("ep")
		sk := cctx.String("sk")

		var (
			repoDir        string
			absHomeDir     string
			configFilePath string
			cfg            *config.Config
			keyfile        string
			selfAdderss    common.Address
		)

		//===============
		// get repoDir's full path
		repoDir = cctx.String(FlagNodeRepo)
		absHomeDir, err = homedir.Expand(repoDir)
		if err != nil {
			return err
		}
		// read config file
		configFilePath = filepath.Join(absHomeDir, "config.json")
		cfg, err = config.ReadFile(configFilePath)
		if err != nil { // read failed
			// read config file failed, endpoint and sk must be given
			if ep == "" || sk == "" {
				return xerrors.Errorf("read config file failed, endpoint and sk must be given")
			}
		} else { // read config file succeed
			// read self wallet from config file
			ar, err := address.NewFromString(cfg.Wallet.DefaultAddress)
			if err != nil {
				return xerrors.Errorf("failed to parse addr %s %w", cfg.Wallet.DefaultAddress, err)
			}
			selfAdderss = common.BytesToAddress(utils.ToEthAddress(ar.Bytes()))
			// trans address format to multiaddr
			maddr, err := address.NewAddress(selfAdderss.Bytes())
			if err != nil {
				return err
			}
			// get keyfile from keystore
			keyfile = filepath.Join(repoDir, "keystore", maddr.String())

			// if no endpoint provided, get ep from config file
			if ep == "" {
				// read endpoint from config file
				ep = cfg.Contract.EndPoint
			}

			// if no sk provided, get sk from api or config file
			if sk == "" {
				sk, err = getSK(cctx, keyfile)
				if err != nil {
					return err
				}
			}
		}
		//===============

		// check payer should be different with receiver
		skECDSA, err := crypto.HexToECDSA(sk)
		if err != nil {
			return err
		}
		pubKey := skECDSA.Public()
		pubKeyECDSA, ok := pubKey.(*ecdsa.PublicKey)
		if !ok {
			return xerrors.New("error casting public key to ECDSA")
		}
		payer := crypto.PubkeyToAddress(*pubKeyECDSA)
		if payer == toAdderss {
			return xerrors.New("payer should be different with receiver")
		}

		// if no receiver provided, transfer to local wallet
		if addr == "0x0" {
			toAdderss = selfAdderss
		}

		// transfer
		return v2.TransferTo(ep, toAdderss, val, sk)
	},
}

var transferErcCmd = &cli.Command{
	Name:      "memo",
	Usage:     "transfer memo, payer's sk can be provided, if not, use wallet address as payer",
	ArgsUsage: "[receiver address (0x...)] [amount (Memo / NanoMemo / AttoMemo)]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "endPoint",
			Aliases: []string{"ep"},
			Usage:   "endpoint of chain, if not given, local config will be used.",
			Value:   "",
		},
		&cli.StringFlag{
			Name:    "instanceAddr",
			Aliases: []string{"ia"},
			Usage:   "instance address, if not given, local config will be used.",
			Value:   "",
		},
		&cli.StringFlag{
			Name:    "version",
			Aliases: []string{"v"},
			Usage:   "contract version, if not given, local config will be used.",
			Value:   "2",
		},
		&cli.StringFlag{
			Name:    "secretKey",
			Aliases: []string{"sk"},
			Usage:   "secret key of admin, if not given, local config will be used.",
			Value:   "",
		},
	},
	Action: func(cctx *cli.Context) error {
		fmt.Println("Transfering memo..")

		if cctx.Args().Len() != 2 {
			return xerrors.Errorf("need two parameters: address and value")
		}

		// get toAddress from param
		addr := cctx.Args().Get(0)
		toAdderss := common.HexToAddress(addr)
		// get val from param
		val, err := types.ParsetValue(cctx.Args().Get(1))
		if err != nil {
			return xerrors.Errorf("parsing 'amount' argument: %w", err)
		}
		if val.Cmp(big.NewInt(0)) == 0 {
			return xerrors.Errorf("amount can't be zero.")
		}

		ep := cctx.String("ep")
		sk := cctx.String("sk")
		ia := cctx.String("ia")
		ver := cctx.Int("v")

		var (
			repoDir        string
			absHomeDir     string
			configFilePath string
			cfg            *config.Config
			keyfile        string
			selfAdderss    common.Address
			instAddr       common.Address
		)

		//===============
		// get repoDir's full path
		repoDir = cctx.String(FlagNodeRepo)
		absHomeDir, err = homedir.Expand(repoDir)
		if err != nil {
			return err
		}
		// read config file
		configFilePath = filepath.Join(absHomeDir, "config.json")
		cfg, err = config.ReadFile(configFilePath)
		if err != nil { // read failed
			// read config file failed, endpoint, sk and instance must be given
			if ep == "" || sk == "" || ia == "" {
				return xerrors.Errorf("read config file failed, endpoint, sk and instance must be given")
			}
		} else { // read config file succeed
			// read self wallet from config file
			ar, err := address.NewFromString(cfg.Wallet.DefaultAddress)
			if err != nil {
				return xerrors.Errorf("failed to parse addr %s %w", cfg.Wallet.DefaultAddress, err)
			}
			selfAdderss = common.BytesToAddress(utils.ToEthAddress(ar.Bytes()))
			// trans address format to multiaddr
			maddr, err := address.NewAddress(selfAdderss.Bytes())
			if err != nil {
				return err
			}
			// get keyfile from keystore
			keyfile = filepath.Join(repoDir, "keystore", maddr.String())

			// get ep from config file
			if ep == "" {
				// read endpoint from config file
				ep = cfg.Contract.EndPoint
			}

			// get instance address from option
			if ia == "" {
				// read instance address from config file
				instAddr = common.HexToAddress(cfg.Contract.RoleContract)
			} else {
				instAddr = common.HexToAddress(ia)
			}

			// if no sk provided, restore sk from api or config file
			if sk == "" {
				sk, err = getSK(cctx, keyfile)
				if err != nil {
					return err
				}
			}
		}
		//===============

		// check payer
		skECDSA, err := crypto.HexToECDSA(sk)
		if err != nil {
			return err
		}
		pubKey := skECDSA.Public()
		pubKeyECDSA, ok := pubKey.(*ecdsa.PublicKey)
		if !ok {
			return xerrors.New("error casting public key to ECDSA")
		}
		payer := crypto.PubkeyToAddress(*pubKeyECDSA)
		if payer == toAdderss {
			return xerrors.New("payer should be different with receiver")
		}

		// if no receiver provided, transfer to self
		if addr == "0x0" {
			toAdderss = selfAdderss
		}

		// call transfer memo
		if ver == 0 {
			rtAddr, err := settle.GetRoleTokenAddr(ep, instAddr, toAdderss)
			if err != nil {
				return err
			}

			tAddr, err := settle.GetTokenAddr(ep, rtAddr, toAdderss, 0)
			if err != nil {
				return err
			}
			return settle.TransferMemoTo(ep, sk, tAddr, toAdderss, val)
		} else {
			tAddr, err := v2.GetTokenAddr(ep, instAddr, sk)
			if err != nil {
				return err
			}
			fmt.Println("token addr:", tAddr)

			return v2.TransferMemoTo(ep, sk, tAddr, toAdderss, val)
		}

	},
}

// get sk from api first, if fail get sk from keystore
func getSK(cctx *cli.Context, keyfile string) (string, error) {
	// input wallet password in commandline
	pw, err := minit.GetPassWord()
	if err != nil {
		return "", err
	}

	// get sk from config file with password
	sk, err := keystore.LoadKeyFile(pw, keyfile)
	if err != nil {
		return "", err
	}

	return sk, nil
}
