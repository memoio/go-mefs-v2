package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/memoio/go-mefs-v2/api/client"
	"github.com/memoio/go-mefs-v2/lib/repo"
	netutils "github.com/memoio/go-mefs-v2/lib/utils/net"
	"github.com/urfave/cli/v2"
)

var bootstrapCmd = &cli.Command{
	Name:  "bootstrap",
	Usage: "bootstrap",
	Subcommands: []*cli.Command{
		bootstrapListCmd,
		bootstrapAddCmd,
		bootstrapClearCmd,
	},
}

var bootstrapListCmd = &cli.Command{
	Name:  "list",
	Usage: "list bootstrap addresses",
	Action: func(cctx *cli.Context) error {
		repoDir := cctx.String(FlagNodeRepo)
		addr, headers, err := client.GetMemoClientInfo(repoDir)
		if err == nil {
			api, closer, err := client.NewGenericNode(cctx.Context, addr, headers)
			if err != nil {
				return err
			}
			defer closer()

			res, err := api.ConfigGet(cctx.Context, "bootstrap.addresses")
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

			fmt.Println("bootstrap addresses: ", out.String())
		} else {
			rep, err := repo.NewFSRepo(repoDir, nil)
			if err != nil {
				return err
			}

			defer rep.Close()

			res := rep.Config().Bootstrap.Addresses
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

			fmt.Println("bootstrap addresses: ", out.String())
		}

		return nil
	},
}

var bootstrapAddCmd = &cli.Command{
	Name:      "add",
	Usage:     "add bootstrap addresses",
	ArgsUsage: "[peer multiaddr (/ip4/1.2.3.4/tcp/5678/p2p/12D...)]",
	Action: func(cctx *cli.Context) error {
		peerAddrs := cctx.Args().Slice()
		_, err := netutils.ParseAddresses(peerAddrs)
		if err != nil {
			return err
		}

		repoDir := cctx.String(FlagNodeRepo)

		rep, err := repo.NewFSRepo(repoDir, nil)
		if err != nil {
			return err
		}

		defer rep.Close()

		baddr := rep.Config().Bootstrap.Addresses

		addedMap := map[string]struct{}{}
		addedList := make([]string, 0, len(baddr)+len(peerAddrs))

		// filter exist
		for _, s := range baddr {
			if _, found := addedMap[s]; found {
				continue
			}
			addedMap[s] = struct{}{}
			addedList = append(addedList, s)
		}

		// filter added
		for _, s := range peerAddrs {
			if _, found := addedMap[s]; found {
				continue
			}
			addedMap[s] = struct{}{}
			addedList = append(addedList, s)
		}

		rep.Config().Bootstrap.Addresses = addedList

		err = rep.ReplaceConfig(rep.Config())
		if err != nil {
			return err
		}

		res := rep.Config().Bootstrap.Addresses
		bs, err := json.MarshalIndent(res, "", "\t")
		if err != nil {
			return err
		}

		var out bytes.Buffer
		err = json.Indent(&out, bs, "", "\t")
		if err != nil {
			return err
		}

		fmt.Println("bootstrap addresses: ", out.String())

		return nil
	},
}

var bootstrapClearCmd = &cli.Command{
	Name:  "clear",
	Usage: "remove all bootstrap addresses",
	Action: func(cctx *cli.Context) error {
		peerAddrs := cctx.Args().Slice()
		_, err := netutils.ParseAddresses(peerAddrs)
		if err != nil {
			return err
		}

		repoDir := cctx.String(FlagNodeRepo)

		rep, err := repo.NewFSRepo(repoDir, nil)
		if err != nil {
			return err
		}

		defer rep.Close()

		addedList := make([]string, 0, 1)

		rep.Config().Bootstrap.Addresses = addedList

		err = rep.ReplaceConfig(rep.Config())
		if err != nil {
			return err
		}

		res := rep.Config().Bootstrap.Addresses
		bs, err := json.MarshalIndent(res, "", "\t")
		if err != nil {
			return err
		}

		var out bytes.Buffer
		err = json.Indent(&out, bs, "", "\t")
		if err != nil {
			return err
		}

		fmt.Println("bootstrap addresses: ", out.String())

		return nil
	},
}
