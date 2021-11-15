package lfscmd

import (
	"fmt"

	"github.com/memoio/go-mefs-v2/api/client"
	"github.com/memoio/go-mefs-v2/app/cmd"
	"github.com/memoio/go-mefs-v2/lib/code"
	"github.com/urfave/cli/v2"
)

var LfsCmd = &cli.Command{
	Name:  "lfs",
	Usage: "Interact with lfs",
	Subcommands: []*cli.Command{
		createBucketCmd,
		listBucketsCmd,
		headBucketCmd,
		putObjectCmd,
		headObjectCmd,
		getObjectCmd,
	},
}

var createBucketCmd = &cli.Command{
	Name:  "create",
	Usage: "create bucket",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "bucket",
			Usage: "bucketName",
		},
	},
	Action: func(cctx *cli.Context) error {
		repoDir := cctx.String(cmd.FlagNodeRepo)
		addr, headers, err := client.GetMemoClientInfo(repoDir)
		if err != nil {
			return err
		}

		napi, closer, err := client.NewUserNode(cctx.Context, addr, headers)
		if err != nil {
			return err
		}
		defer closer()

		bucketName := cctx.String("bucket")

		bi, err := napi.CreateBucket(cctx.Context, bucketName, code.DefaultBucketOptions())
		if err != nil {
			return err
		}

		fmt.Println("create bucket: ", bi.BucketID, bi.Name)

		return nil
	},
}

var listBucketsCmd = &cli.Command{
	Name:  "list",
	Usage: "list buckets",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "prefix",
			Usage: "bucket prefix",
		},
	},
	Action: func(cctx *cli.Context) error {
		repoDir := cctx.String(cmd.FlagNodeRepo)
		addr, headers, err := client.GetMemoClientInfo(repoDir)
		if err != nil {
			return err
		}

		napi, closer, err := client.NewUserNode(cctx.Context, addr, headers)
		if err != nil {
			return err
		}
		defer closer()

		prefix := cctx.String("prefix")

		bs, err := napi.ListBuckets(cctx.Context, prefix)
		if err != nil {
			return err
		}

		fmt.Println("list buckets: ")
		for _, bi := range bs {
			fmt.Println(bi.BucketID, bi.Name, bi.DataCount, bi)
		}

		return nil
	},
}

var headBucketCmd = &cli.Command{
	Name:  "head",
	Usage: "head bucket info",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "bucket",
			Usage: "bucketName",
		},
	},
	Action: func(cctx *cli.Context) error {
		repoDir := cctx.String(cmd.FlagNodeRepo)
		addr, headers, err := client.GetMemoClientInfo(repoDir)
		if err != nil {
			return err
		}

		napi, closer, err := client.NewUserNode(cctx.Context, addr, headers)
		if err != nil {
			return err
		}
		defer closer()

		bucketName := cctx.String("bucket")

		bi, err := napi.HeadBucket(cctx.Context, bucketName)
		if err != nil {
			return err
		}

		fmt.Println("create bucket: ", bi.BucketID, bi.Name, bi)

		return nil
	},
}
