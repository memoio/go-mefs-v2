package cmd

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/memoio/go-mefs-v2/api/client"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var RestrictCmd = &cli.Command{
	Name:  "restrict",
	Usage: "Interact with restrict",
	Subcommands: []*cli.Command{
		restrictListCmd,
		restrictAddCmd,
		restrictDeleteCmd,
		restrictHasCmd,
		restrictEnableCmd,
		restrictStatCmd,
	},
}

var restrictListCmd = &cli.Command{
	Name:  "list",
	Usage: "list all accepted users/providers",
	Action: func(cctx *cli.Context) error {
		repoDir := cctx.String(FlagNodeRepo)
		addr, headers, err := client.GetMemoClientInfo(repoDir)
		if err != nil {
			return err
		}

		api, closer, err := client.NewUserNode(cctx.Context, addr, headers)
		if err != nil {
			return err
		}
		defer closer()

		ois, err := api.RestrictList(cctx.Context)
		if err != nil {
			return err
		}

		sort.Slice(ois, func(i, j int) bool {
			return ois[i] < ois[j]
		})

		fmt.Println(ois)

		return nil
	},
}

var restrictStatCmd = &cli.Command{
	Name:  "stat",
	Usage: "restrict stat(enable/disable)",
	Action: func(cctx *cli.Context) error {
		repoDir := cctx.String(FlagNodeRepo)
		addr, headers, err := client.GetMemoClientInfo(repoDir)
		if err != nil {
			return err
		}

		api, closer, err := client.NewUserNode(cctx.Context, addr, headers)
		if err != nil {
			return err
		}
		defer closer()

		eflag, err := api.RestrictStat(cctx.Context)
		if err != nil {
			return err
		}
		if eflag {
			fmt.Println("restrict is enabled")
		} else {
			fmt.Println("restrict is disabled")
		}

		return nil
	},
}

var restrictEnableCmd = &cli.Command{
	Name:  "set",
	Usage: "enable/disable restrict",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "enable",
			Usage: "enable/disable",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		repoDir := cctx.String(FlagNodeRepo)
		addr, headers, err := client.GetMemoClientInfo(repoDir)
		if err != nil {
			return err
		}

		api, closer, err := client.NewUserNode(cctx.Context, addr, headers)
		if err != nil {
			return err
		}
		defer closer()

		eflag := cctx.Bool("enable")
		err = api.RestrictEnable(cctx.Context, eflag)
		if err != nil {
			return err
		}
		if eflag {
			fmt.Println("restrict is enabled")
		} else {
			fmt.Println("restrict is disabled")
		}

		return nil
	},
}

var restrictAddCmd = &cli.Command{
	Name:      "add",
	Usage:     "add node(s) to restrict list",
	ArgsUsage: "node ids",
	Action: func(cctx *cli.Context) error {
		ids := cctx.Args().Slice()

		if len(ids) < 1 {
			return xerrors.Errorf("input at lease one parameter")
		}

		repoDir := cctx.String(FlagNodeRepo)
		addr, headers, err := client.GetMemoClientInfo(repoDir)
		if err != nil {
			return err
		}

		api, closer, err := client.NewUserNode(cctx.Context, addr, headers)
		if err != nil {
			return err
		}
		defer closer()

		for _, id := range ids {
			nid, err := strconv.Atoi(id)
			if err != nil {
				continue
			}
			err = api.RestrictAdd(cctx.Context, uint64(nid))
			if err != nil {
				return err
			}

		}

		return nil
	},
}

var restrictDeleteCmd = &cli.Command{
	Name:      "delete",
	Usage:     "remove node(s) from restrict list",
	ArgsUsage: "node ids",
	Action: func(cctx *cli.Context) error {
		ids := cctx.Args().Slice()

		if len(ids) < 1 {
			return xerrors.Errorf("input at lease one parameter")
		}

		repoDir := cctx.String(FlagNodeRepo)
		addr, headers, err := client.GetMemoClientInfo(repoDir)
		if err != nil {
			return err
		}

		api, closer, err := client.NewUserNode(cctx.Context, addr, headers)
		if err != nil {
			return err
		}
		defer closer()

		for _, id := range ids {
			nid, err := strconv.Atoi(id)
			if err != nil {
				continue
			}
			err = api.RestrictDelete(cctx.Context, uint64(nid))
			if err != nil {
				return err
			}

		}

		return nil
	},
}

var restrictHasCmd = &cli.Command{
	Name:      "has",
	Usage:     "test whether node(s) in restrict list",
	ArgsUsage: "node ids",
	Action: func(cctx *cli.Context) error {
		ids := cctx.Args().Slice()

		if len(ids) < 1 {
			return xerrors.Errorf("input at lease one parameter")
		}

		repoDir := cctx.String(FlagNodeRepo)
		addr, headers, err := client.GetMemoClientInfo(repoDir)
		if err != nil {
			return err
		}

		api, closer, err := client.NewUserNode(cctx.Context, addr, headers)
		if err != nil {
			return err
		}
		defer closer()

		for _, id := range ids {
			nid, err := strconv.Atoi(id)
			if err != nil {
				continue
			}
			has := api.RestrictHas(cctx.Context, uint64(nid))
			if has {
				fmt.Println("restrict list has: ", nid)
			} else {
				fmt.Println("restrict list has not: ", nid)
			}

		}

		return nil
	},
}
