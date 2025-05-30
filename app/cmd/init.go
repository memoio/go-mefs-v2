package cmd

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/app/minit"
	"github.com/memoio/go-mefs-v2/config"
	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/backend/keystore"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/repo"
	"github.com/memoio/go-mefs-v2/lib/utils"
)

var logger = logging.Logger("cmd")

const (
	FlagNodeRepo = "repo"
	FlagRoleType = "roleType"
)

// new repo and create wallet for you
var initCmd = &cli.Command{
	Name:  "init",
	Usage: "Initialize a memoriae repo",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "setPass",
			Usage: "set password using input",
			Value: false,
		},
		&cli.StringFlag{
			Name:    pwKwd,
			Aliases: []string{"pwd"},
			Usage:   "set password for access secret key",
			Value:   "memoriae",
		},
		&cli.StringFlag{
			Name:    "secretKey",
			Aliases: []string{"sk"},
			Usage:   "secret key",
			Value:   "",
		},
		&cli.StringFlag{
			Name:    "keyfile",
			Aliases: []string{"kf"},
			Usage:   "absolute path of keyfile",
			Value:   "",
		},
		&cli.StringFlag{
			Name:  "kpw",
			Usage: "password to decrypt keyfile",
			Value: "",
		},
	},
	Action: func(cctx *cli.Context) error {
		logger.Info("initializing memoriae node")

		pw := cctx.String(pwKwd)
		setpass := cctx.Bool("setPass")
		if setpass {
			npw, err := minit.GetPassWord()
			if err != nil {
				if len(npw) > 0 && len(npw) < 8 {
					return xerrors.Errorf("password length should be at least 8")
				}
			}
			pw = npw
		}
		logger.Info("check if repo exists")

		repoDir := cctx.String(FlagNodeRepo)

		exist, err := repo.Exists(repoDir)
		if err != nil {
			return err
		}
		if exist {
			return xerrors.Errorf("repo at '%s' is already initialized", repoDir)
		}

		logger.Info("initializing repo at: ", repoDir)

		// new repo
		rep, err := repo.NewFSRepo(repoDir, config.NewDefaultConfig(), true)
		if err != nil {
			return err
		}

		defer func() {
			_ = rep.Close()
		}()

		rType := cctx.String(FlagRoleType)
		rep.Config().Identity.Role = rType

		// from key file
		kf := cctx.String("kf")
		if kf != "" {
			kpw := cctx.String("kpw")
			sk, err := keystore.LoadKeyFile(kpw, kf)
			if err != nil {
				return err
			}

			err = minit.Create(cctx.Context, rep, pw, sk)
			if err != nil {
				logger.Errorf("fail initializing node %s", err)
				return err
			}

			return nil
		}

		// from secret key
		sk := cctx.String("sk")
		err = minit.Create(cctx.Context, rep, pw, sk)
		if err != nil {
			logger.Errorf("fail initializing node %s", err)
			return err
		}

		// show wallet address
		w := rep.Config().Wallet
		addr, err := address.NewFromString(w.DefaultAddress)
		if err != nil {
			return xerrors.Errorf("failed to parse addr %s %w", w.DefaultAddress, err)
		}
		ethAddress := common.BytesToAddress(utils.ToEthAddress(addr.Bytes()))

		fmt.Println()
		fmt.Printf("wallet address: %s\n", ethAddress)
		fmt.Println()

		return nil
	},
}
