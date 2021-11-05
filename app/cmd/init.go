package cmd

import (
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/config"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	minit "github.com/memoio/go-mefs-v2/lib/minit"
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

		if err := repo.InitFSRepo(repoDir, repo.LatestVersion, config.NewDefaultConfig()); err != nil {
			return err
		}

		if err = InitRun(cctx); err != nil {
			return err
		}

		return nil
	},
}

func InitRun(cctx *cli.Context) error {
	repoDir := cctx.String(FlagNodeRepo)
	rep, err := OpenRepo(repoDir)
	if err != nil {
		return err
	}
	// The only error Close can return is that the repo has already been closed.
	defer func() {
		_ = rep.Close()
	}()

	rType := cctx.String(FlagRoleType)
	rep.Config().Identity.Role = rType

	password := cctx.String("password")

	if err := minit.Init(cctx.Context, rep, password); err != nil {
		logger.Errorf("Error initializing node %s", err)
		return err
	}

	if err := rep.ReplaceConfig(rep.Config()); err != nil {
		logger.Errorf("Error replacing config %s", err)
		return err
	}

	return nil
}

func OpenRepo(repoDir string) (repo.Repo, error) {
	return repo.OpenFSRepo(repoDir, repo.LatestVersion)
}
