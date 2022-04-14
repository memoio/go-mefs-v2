package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

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
			Usage: "path to store",
		},
		&cli.StringFlag{
			Name:  "dbType",
			Usage: "export dbtype: meta or state",
			Value: "state",
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
		fi, err := os.Stat(p)
		if err != nil {
			return err
		}

		if !fi.IsDir() {
			return xerrors.Errorf("%s should be dir", p)
		}

		dir := filepath.Dir(p)

		dbType := cctx.String("dbType")
		if dbType != "state" && dbType != "meta" {
			return xerrors.Errorf("%s not supported, 'meta' or 'state'", dbType)
		}

		p = filepath.Join(dir, dbType+"-"+time.Now().Format("20060102T150405")+".db")

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

		bar := progressbar.DefaultBytes(-1, "export:")

		stateDir := filepath.Join(repoDir, dbType)
		opt := badger.DefaultOptions(stateDir).WithReadOnly(true)
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

		fmt.Printf("finish export %s to %s\n", dbType, p)
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
		&cli.StringFlag{
			Name:  "dbType",
			Usage: "export dbtype: meta or state",
			Value: "state",
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

		fi, err := pf.Stat()
		if err != nil {
			return err
		}

		bar := progressbar.DefaultBytes(fi.Size(), "import:")

		dbType := cctx.String("dbType")
		if dbType != "state" && dbType != "meta" {
			return xerrors.Errorf("%s not supported, 'meta' or 'state'", dbType)
		}

		if !strings.Contains(fi.Name(), dbType) {
			return xerrors.Errorf("check your path, should have %s", dbType)
		}

		stateDir := filepath.Join(repoDir, dbType)
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

		fmt.Printf("finish import %s from %s\n", dbType, p)

		return nil
	},
}
