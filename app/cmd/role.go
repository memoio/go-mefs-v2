package cmd

import (
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/memoio/go-mefs-v2/api/client"
	"github.com/memoio/go-mefs-v2/lib/pb"
)

var roleCmd = &cli.Command{
	Name:  "role",
	Usage: "Interact with role manager",
	Subcommands: []*cli.Command{
		roleListCmd,
	},
}

var roleListCmd = &cli.Command{
	Name:  "list",
	Usage: "list connected roles",
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

		pros, err := napi.RoleGetRelated(cctx.Context, pb.RoleInfo_Provider)
		if err != nil {
			return err
		}
		fmt.Println(pros)

		return nil
	},
}
