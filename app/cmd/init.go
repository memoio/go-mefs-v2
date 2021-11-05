package cmd

import (
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/app/minit"
	"github.com/memoio/go-mefs-v2/config"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/repo"
)

var logger = logging.Logger("main")

const (
	FlagNodeRepo = "repo"
	FlagRoleType = "roleType"
)

var InitCmd = &cli.Command{
	Name:  "init",
	Usage: "Initialize a memoriae repo",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "password",
			Usage: "password for aaacess private key",
			Value: "memoriae",
		},
	},
	Action: func(cctx *cli.Context) error {
		logger.Info("Initializing memoriae node")

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

		password := cctx.String("password")

		if err := minit.Create(cctx.Context, rep, password); err != nil {
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
