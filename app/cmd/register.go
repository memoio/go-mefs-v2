package cmd

import (
	"fmt"
	callconts "memoc/callcontracts"
	"path/filepath"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/memoio/go-mefs-v2/app/minit"
	"github.com/memoio/go-mefs-v2/config"
	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/backend/keystore"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/submodule/connect/settle"
	v2 "github.com/memoio/go-mefs-v2/submodule/connect/v2"
	"github.com/memoio/go-mefs-v2/submodule/wallet"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var registerCmd = &cli.Command{
	Name:      "register",
	Usage:     "register role",
	ArgsUsage: "[role (user/keeper/provider)] [group index]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    pwKwd,
			Aliases: []string{"pwd"},
			Value:   "memoriae",
		},
		&cli.StringFlag{
			Name:  "roleContract",
			Usage: "address role contract",
			Value: callconts.RoleAddr.String(),
		},
		&cli.IntFlag{
			Name:  "version",
			Usage: "contract version",
			Value: 0,
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 2 {
			return xerrors.Errorf("need twp paras:role type, group index")
		}

		gid, err := strconv.ParseUint(cctx.Args().Get(1), 10, 0)
		if err != nil {
			return xerrors.Errorf("parsing 'group index' argument: %w", err)
		}

		repoDir := cctx.String(FlagNodeRepo)
		absHomeDir, err := homedir.Expand(repoDir)
		if err != nil {
			return err
		}

		configFile := filepath.Join(absHomeDir, "config.json")
		cfg, err := config.ReadFile(configFile)
		if err != nil {
			return xerrors.Errorf("failed to read config file at %q %w", configFile, err)
		}

		ar, err := address.NewFromString(cfg.Wallet.DefaultAddress)
		if err != nil {
			return xerrors.Errorf("failed to parse addr %s %w", cfg.Wallet.DefaultAddress, err)
		}

		ksp := filepath.Join(absHomeDir, "keystore")

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

		typ := pb.RoleInfo_Unknown
		switch cctx.Args().Get(0) {
		case "user":
			typ = pb.RoleInfo_User
		case "provider":
			typ = pb.RoleInfo_Provider
		case "keeper":
			typ = pb.RoleInfo_Keeper
		}

		rAddr := common.HexToAddress(cfg.Contract.RoleContract)
		ep := cfg.Contract.EndPoint
		ver := int(cfg.Contract.Version)

		var uid uint64
		if ver == 0 {
			uid, gid, err = settle.Register(cctx.Context, cfg.Contract.EndPoint, rAddr.String(), ki.SecretKey, typ, gid)
			if err != nil {
				return err
			}
		} else {
			rid, gid, err := v2.Register(cctx.Context, ep, rAddr.String(), []byte(ki.SecretKey), typ, gid)
			fmt.Println("after register, rid, gid: ", rid, gid)
			return err
		}

		fmt.Printf("register as %d in group %d", uid, gid)

		return nil
	},
}
