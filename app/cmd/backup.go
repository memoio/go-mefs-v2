package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/dgraph-io/badger/v2"
	"github.com/mitchellh/go-homedir"
	"github.com/schollz/progressbar/v3"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/lib/repo"
)

var backupCmd = &cli.Command{
	Name:  "backup",
	Usage: "backup export or import",
	Subcommands: []*cli.Command{
		backupExportCmd,
		backupImportCmd,
	},
}

var backupExportCmd = &cli.Command{
	Name:  "export",
	Usage: "export state db to file",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "path",
			Usage: "path of file to store",
			Value: "",
		},
	},
	Action: func(cctx *cli.Context) error {
		// test repo is lock?
		repoDir := cctx.String(FlagNodeRepo)
		rep, err := repo.NewFSRepo(repoDir, nil)
		if err != nil {
			return err
		}
		rep.Close()

		path := cctx.String("path")
		// get full path of home dir
		p, err := homedir.Expand(path)
		if err != nil {
			return err
		}

		p, err = filepath.Abs(p)
		if err != nil {
			return err
		}

		// check file exist

		_, error := os.Stat(p)
		// check if error is "file not exists"
		if !os.IsNotExist(error) {
			return xerrors.Errorf("%s exist", p)
		}

		pf, err := os.Create(p)
		if err != nil {
			return err
		}
		defer pf.Close()

		bar := progressbar.DefaultBytes(-1, "export")

		stateDir := filepath.Join(repoDir, "state")
		opt := badger.DefaultOptions(stateDir)
		opt = opt.WithLoggingLevel(badger.ERROR)
		db, err := badger.Open(opt)
		if err != nil {
			return err
		}

		_, err = db.Backup(io.MultiWriter(pf, bar), 0)
		if err != nil {
			return err
		}

		bar.Finish()

		fmt.Printf("export to %s\n", p)
		return nil
	},
}

var backupImportCmd = &cli.Command{
	Name:  "import",
	Usage: "import from file",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "path",
			Usage: "path of file import from",
			Value: "",
		},
	},
	Action: func(cctx *cli.Context) error {
		// test repo is lock?
		repoDir := cctx.String(FlagNodeRepo)
		rep, err := repo.NewFSRepo(repoDir, nil)
		if err != nil {
			return err
		}
		rep.Close()

		path := cctx.String("path")
		// get full path of home dir
		p, err := homedir.Expand(path)
		if err != nil {
			return err
		}

		p, err = filepath.Abs(p)
		if err != nil {
			return err
		}

		// open repo dir
		pf, err := os.Open(p)
		if err != nil {
			return err
		}
		defer pf.Close()

		bar := progressbar.DefaultBytes(-1, "import")

		stateDir := filepath.Join(repoDir, "state")
		opt := badger.DefaultOptions(stateDir)
		opt = opt.WithLoggingLevel(badger.ERROR)
		db, err := badger.Open(opt)
		if err != nil {
			return err
		}

		pr := progressbar.NewReader(pf, bar)

		err = db.Load(&pr, 256)
		if err != nil {
			return err
		}

		bar.Finish()

		fmt.Printf("import from %s\n", p)

		return nil
	},
}
