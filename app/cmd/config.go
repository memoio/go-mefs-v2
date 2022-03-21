package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/memoio/go-mefs-v2/lib/repo"
	"github.com/urfave/cli/v2"
)

var ConfigCmd = &cli.Command{
	Name:  "config",
	Usage: "Interact with config",
	Subcommands: []*cli.Command{
		configSetCmd,
		configGetCmd,
	},
}

var configGetCmd = &cli.Command{
	Name:  "get",
	Usage: "Get config key",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "key",
			Usage: "The key of the config entry (e.g. \"api.address\")",
			Value: "",
		},
	},
	Action: func(cctx *cli.Context) error {
		repoDir := cctx.String(FlagNodeRepo)
		rep, err := repo.NewFSRepo(repoDir, nil)
		if err != nil {
			return err
		}

		defer rep.Close()

		key := cctx.String("key")
		if key == "" {
			return errors.New("key is nil")
		}

		res, err := rep.Config().Get(key)
		if err != nil {
			return err
		}

		bs, err := json.MarshalIndent(res, "", "\t")
		if err != nil {
			return err
		}

		var out bytes.Buffer
		err = json.Indent(&out, bs, "", "\t")
		if err != nil {
			return err
		}

		fmt.Printf("%v\n", out.String())

		return nil
	},
}

var configSetCmd = &cli.Command{
	Name:  "set",
	Usage: "Set config key",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "key",
			Usage: "The key of the config entry (e.g. \"api.address\")",
			Value: "",
		},
		&cli.StringFlag{
			Name:  "value",
			Usage: "The value with which to set the config entry",
			Value: "",
		},
	},
	Action: func(cctx *cli.Context) error {
		repoDir := cctx.String(FlagNodeRepo)
		rep, err := repo.NewFSRepo(repoDir, nil)
		if err != nil {
			return err
		}

		defer rep.Close()

		key := cctx.String("key")
		if key == "" {
			return errors.New("key is nil")
		}

		value := cctx.String("value")

		err = rep.Config().Set(key, value)
		if err != nil {
			return err
		}

		err = rep.ReplaceConfig(rep.Config())
		if err != nil {
			logger.Errorf("Error replacing config %s", err)
			return err
		}

		res, err := rep.Config().Get(key)
		if err != nil {
			return err
		}

		bs, err := json.MarshalIndent(res, "", "\t")
		if err != nil {
			return err
		}

		var out bytes.Buffer
		err = json.Indent(&out, bs, "", "\t")
		if err != nil {
			return err
		}

		fmt.Printf("%v\n", out.String())

		return nil
	},
}
