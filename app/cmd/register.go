package cmd

import (
	"path/filepath"
	"strconv"

	"github.com/memoio/go-mefs-v2/app/minit"
	"github.com/memoio/go-mefs-v2/config"
	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/backend/keystore"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/submodule/connect/settle"
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

		cm, err := settle.NewContractMgr(cctx.Context, cfg.Contract.EndPoint, cfg.Contract.RoleContract, ki.SecretKey)
		if err != nil {
			return err
		}

		switch cctx.Args().Get(0) {
		case "user":
			return cm.Start(pb.RoleInfo_User, gid)
		case "provider":
			return cm.Start(pb.RoleInfo_Provider, gid)
		case "keeper":
			return cm.Start(pb.RoleInfo_Keeper, gid)
		default:
			return cm.Start(pb.RoleInfo_Unknown, gid)
		}
	},
}
