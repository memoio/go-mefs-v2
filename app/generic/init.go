package generic

import (
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/config"
	minit "github.com/memoio/go-mefs-v2/lib/init"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/repo"
)

var log = logging.Logger("main")

const FlagNodeRepo = "repo"

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
		log.Info("Initializing memoriae node")

		log.Info("Checking if repo exists")

		repoDir := cctx.String(FlagNodeRepo)

		exist, err := repo.Exists(repoDir)
		if err != nil {
			return err
		}
		if exist {
			return xerrors.Errorf("repo at '%s' is already initialized", repoDir)
		}

		log.Infof("Initializing repo at '%s'", repoDir)

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

	password := cctx.String("password")

	if err := minit.Init(cctx.Context, rep, password); err != nil {
		log.Errorf("Error initializing node %s", err)
		return err
	}

	if err := rep.ReplaceConfig(rep.Config()); err != nil {
		log.Errorf("Error replacing config %s", err)
		return err
	}

	return nil
}

func OpenRepo(repoDir string) (repo.Repo, error) {
	return repo.OpenFSRepo(repoDir, repo.LatestVersion)
}
