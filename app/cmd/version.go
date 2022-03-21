package cmd

import (
	"fmt"

	"github.com/memoio/go-mefs-v2/api/client"
	"github.com/urfave/cli/v2"
)

var versionCmd = &cli.Command{
	Name:  "version",
	Usage: "Print version",
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

		ver, err := napi.Version(cctx.Context)
		if err != nil {
			return err
		}

		fmt.Println("Daemon: ", ver)
		fmt.Print("Local: ")

		cli.VersionPrinter(cctx)
		return nil
	},
}
