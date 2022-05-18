package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/memoio/go-mefs-v2/api/client"
	"github.com/memoio/go-mefs-v2/lib/repo"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var configCmd = &cli.Command{
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
		key := cctx.String("key")
		if key == "" {
			return xerrors.New("key is nil")
		}

		repoDir := cctx.String(FlagNodeRepo)
		addr, headers, err := client.GetMemoClientInfo(repoDir)
		if err == nil {
			api, closer, err := client.NewGenericNode(cctx.Context, addr, headers)
			if err != nil {
				return err
			}
			defer closer()

			res, err := api.ConfigGet(cctx.Context, key)
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

			fmt.Printf("get key: %s, value: %v\n", key, out.String())
		} else {
			rep, err := repo.NewFSRepo(repoDir, nil)
			if err != nil {
				return err
			}

			defer rep.Close()

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

			fmt.Printf("get %s value %v\n", key, out.String())
		}

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
		key := cctx.String("key")
		if key == "" {
			return xerrors.New("key is nil")
		}

		value := cctx.String("value")

		repoDir := cctx.String(FlagNodeRepo)
		addr, headers, err := client.GetMemoClientInfo(repoDir)
		if err == nil {
			api, closer, err := client.NewGenericNode(cctx.Context, addr, headers)
			if err != nil {
				return err
			}
			defer closer()

			err = api.ConfigSet(cctx.Context, key, value)
			if err != nil {
				return err
			}

			fmt.Printf("set %s to %v\n", key, value)
			fmt.Println("It will take affect at next start")
		} else {
			rep, err := repo.NewFSRepo(repoDir, nil)
			if err != nil {
				return err
			}

			defer rep.Close()

			err = rep.Config().Set(key, value)
			if err != nil {
				return err
			}

			err = rep.ReplaceConfig(rep.Config())
			if err != nil {
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

			fmt.Printf("set %s to %v\n", key, out.String())
			fmt.Println("It will take affect at next start")
		}

		return nil
	},
}
