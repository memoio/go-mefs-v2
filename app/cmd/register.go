package cmd

import (
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/app/minit"
	"github.com/memoio/go-mefs-v2/config"
	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/backend/keystore"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/submodule/connect/settle"
	"github.com/memoio/go-mefs-v2/submodule/wallet"
)

// register account id and role
var registerCmd = &cli.Command{
	Name:      "register",
	Usage:     "register an account id for the wallet first, then register a role for it, at last, add it into a group.",
	ArgsUsage: "[role (user/keeper/provider)] [group index]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    pwKwd,
			Usage:   "the wallet password, used to restore sk from keystore",
			Aliases: []string{"pwd"},
			Value:   "memoriae",
		},
	},
	Action: func(cctx *cli.Context) error {
		// need type and group id
		if cctx.Args().Len() != 2 {
			return xerrors.Errorf("need twp paras:role type, group index")
		}

		// get group id in param 1
		gid, err := strconv.ParseUint(cctx.Args().Get(1), 10, 0)
		if err != nil {
			return xerrors.Errorf("parsing 'group index' argument: %w", err)
		}

		// get repo dir
		repoDir := cctx.String(FlagNodeRepo)
		absHomeDir, err := homedir.Expand(repoDir)
		if err != nil {
			return err
		}

		// get config file in repo
		configFile := filepath.Join(absHomeDir, "config.json")
		cfg, err := config.ReadFile(configFile)
		if err != nil {
			return xerrors.Errorf("failed to read config file at %q %w", configFile, err)
		}

		// get wallet address from config
		ar, err := address.NewFromString(cfg.Wallet.DefaultAddress)
		if err != nil {
			return xerrors.Errorf("failed to parse addr %s %w", cfg.Wallet.DefaultAddress, err)
		}

		// get keystore path
		ksp := filepath.Join(absHomeDir, "keystore")

		// get keystore in repo
		ks, err := keystore.NewKeyRepo(ksp)
		if err != nil {
			return err
		}

		// get pw of wallet from option or input
		pw := cctx.String(pwKwd)
		if pw == "" {
			pw, err = minit.GetPassWord()
			if err != nil {
				return err
			}
		}

		// get sk using keystore and pw
		lw := wallet.New(pw, ks)
		ki, err := lw.WalletExport(cctx.Context, ar, pw)
		if err != nil {
			return err
		}

		// get type from param 0
		typ := pb.RoleInfo_Unknown
		switch cctx.Args().Get(0) {
		case "user":
			typ = pb.RoleInfo_User
		case "provider":
			typ = pb.RoleInfo_Provider
		case "keeper":
			typ = pb.RoleInfo_Keeper
		}

		// read info from config
		rAddr := common.HexToAddress(cfg.Contract.RoleContract)
		ep := cfg.Contract.EndPoint
		ver := int(cfg.Contract.Version)

		rid, gid, err := settle.Register(cctx.Context, ep, rAddr.String(), uint32(ver), []byte(ki.SecretKey), typ, gid)
		if err != nil {
			return xerrors.Errorf("contract version: %d is wrong, correct is 0,2,3", cfg.Contract.Version)
		}

		fmt.Printf("register as %d in group %d", rid, gid)

		return nil
	},
}
