package cmd

import (
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/api/client"
)

var authCmd = &cli.Command{
	Name:  "auth",
	Usage: "Interact with auth",
	Subcommands: []*cli.Command{
		authInfoCmd,
	},
}

var authInfoCmd = &cli.Command{
	Name:  "info",
	Usage: "get api info",
	Action: func(cctx *cli.Context) error {
		repoDir := cctx.String(FlagNodeRepo)
		addr, headers, err := client.GetMemoClientInfo(repoDir)
		if err != nil {
			return err
		}

		napi, closer, err := client.NewGenericNode(cctx.Context, addr, headers)
		if err != nil {
			return err
		}
		defer closer()

		token, err := napi.AuthNew(cctx.Context, api.AllPermissions)
		if err != nil {
			return err
		}
		fmt.Println(string(token))

		return nil
	},
}
