package cmd

import (
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/app/minit"
	"github.com/memoio/go-mefs-v2/config"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/repo"
)

var logger = logging.Logger("cmd")

const (
	FlagNodeRepo = "repo"
	FlagRoleType = "roleType"
)

var InitCmd = &cli.Command{
	Name:  "init",
	Usage: "Initialize a memoriae repo",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "setPass",
			Usage: "set password using input",
			Value: false,
		},
		&cli.StringFlag{
			Name:  "password",
			Usage: "set password for access private key",
			Value: "memoriae",
		},
	},
	Action: func(cctx *cli.Context) error {
		logger.Info("Initializing memoriae node")

		pw := cctx.String("password")
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
		logger.Info("Checking if repo exists")

		repoDir := cctx.String(FlagNodeRepo)

		exist, err := repo.Exists(repoDir)
		if err != nil {
			return err
		}
		if exist {
			return xerrors.Errorf("repo at '%s' is already initialized", repoDir)
		}

		logger.Infof("Initializing repo at '%s'", repoDir)

		rep, err := repo.NewFSRepo(repoDir, config.NewDefaultConfig())
		if err != nil {
			return err
		}

		defer func() {
			_ = rep.Close()
		}()

		rType := cctx.String(FlagRoleType)
		rep.Config().Identity.Role = rType

		if err := minit.Create(cctx.Context, rep, pw); err != nil {
			logger.Errorf("Error initializing node %s", err)
			return err
		}

		if err := rep.ReplaceConfig(rep.Config()); err != nil {
			logger.Errorf("Error replacing config %s", err)
			return err
		}

		return nil
	},
}
