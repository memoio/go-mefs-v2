package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dgraph-io/badger/v2"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var recoverCmd = &cli.Command{
	Name:  "recover",
	Usage: "recover",
	Subcommands: []*cli.Command{
		recoverDBCmd,
	},
}

var recoverDBCmd = &cli.Command{
	Name:  "db",
	Usage: "recover db which need truncate",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "path",
			Usage: "db path to open",
			Value: "",
		},
		&cli.StringFlag{
			Name:  "dbType",
			Usage: "db type: meta or state",
			Value: "",
		},
	},
	Action: func(cctx *cli.Context) error {
		dbType := cctx.String("dbType")
		var dbDir string

		if dbType == "" {
			path := cctx.String("path")
			if path == "" {
				return xerrors.Errorf("set path or type")
			}
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

			dbDir = p
		} else {
			repoDir := cctx.String(FlagNodeRepo)
			if dbType != "state" && dbType != "meta" {
				return xerrors.Errorf("%s not supported, 'meta' or 'state'", dbType)
			}
			dbDir = filepath.Join(repoDir, dbType)
		}

		opt := badger.DefaultOptions(dbDir).WithTruncate(true)
		db, err := badger.Open(opt)
		if err != nil {
			return err
		}

		db.Sync()
		db.Close()

		fmt.Printf("finish recover db in %s\n", dbDir)
		return nil
	},
}
