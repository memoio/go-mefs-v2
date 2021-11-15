package lfscmd

import (
	"fmt"
	"os"

	"github.com/memoio/go-mefs-v2/api/client"
	"github.com/memoio/go-mefs-v2/app/cmd"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
)

var putObjectCmd = &cli.Command{
	Name:  "put",
	Usage: "put object",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "bucket",
			Usage: "bucketName",
		},
		&cli.StringFlag{
			Name:  "object",
			Usage: "objectName",
		},
		&cli.StringFlag{
			Name:  "path",
			Usage: "path of file",
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
		objectName := cctx.String("object")
		path := cctx.String("path")

		p, err := homedir.Expand(path)
		if err != nil {
			return err
		}

		pf, err := os.Open(p)
		if err != nil {
			return err
		}
		defer pf.Close()

		poo := types.PutObjectOptions{}

		oi, err := napi.PutObject(cctx.Context, bucketName, objectName, pf, poo)
		if err != nil {
			return err
		}

		fmt.Println("put object: ", oi.State)

		return nil
	},
}

var headObjectCmd = &cli.Command{
	Name:  "head",
	Usage: "head object",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "bucket",
			Usage: "bucketName",
		},
		&cli.StringFlag{
			Name:  "object",
			Usage: "objectName",
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
		objectName := cctx.String("object")

		oi, err := napi.HeadObject(cctx.Context, bucketName, objectName)
		if err != nil {
			return err
		}

		fmt.Println("head object: ", oi.ObjectID, oi.Name, oi)

		return nil
	},
}

var getObjectCmd = &cli.Command{
	Name:  "get",
	Usage: "get object",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "bucket",
			Usage: "bucketName",
		},
		&cli.StringFlag{
			Name:  "object",
			Usage: "objectName",
		},
		&cli.StringFlag{
			Name:  "path",
			Usage: "stored path of file",
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
		objectName := cctx.String("object")
		path := cctx.String("path")

		p, err := homedir.Expand(path)
		if err != nil {
			return err
		}

		pf, err := os.Open(p)
		if err != nil {
			return err
		}
		defer pf.Close()

		doo := types.DownloadObjectOptions{}

		err = napi.GetObject(cctx.Context, bucketName, objectName, pf, nil, doo)
		if err != nil {
			return err
		}

		fmt.Println("get object: ", objectName)

		return nil
	},
}
