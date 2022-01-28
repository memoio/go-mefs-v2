package lfscmd

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/memoio/go-mefs-v2/api/client"
	"github.com/memoio/go-mefs-v2/app/cmd"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/utils"
	"github.com/mgutz/ansi"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
)

func FormatObjectInfo(object *types.ObjectInfo) string {
	return fmt.Sprintf(
		`Name: %s
BucketID: %d
ObjectID: %d
Etag: %s
CTime: %s
MTime: %s
Size: %s
State: %s
Enc: %s`,
		ansi.Color(object.Name, "green"),
		object.BucketID,
		object.ObjectID,
		hex.EncodeToString(object.Etag),
		time.Unix(int64(object.Time), 0).Format(utils.SHOWTIME),
		time.Unix(int64(object.Mtime), 0).Format(utils.SHOWTIME),
		utils.FormatBytes(int64(object.Length)),
		object.State,
		object.Encryption,
	)
}

var listObjectsCmd = &cli.Command{
	Name:  "listObjects",
	Usage: "list objects",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "bucket",
			Aliases: []string{"bn"},
			Usage:   "bucketName",
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

		ops := types.DefaultListOption()
		loi, err := napi.ListObjects(cctx.Context, bucketName, ops)
		if err != nil {
			return err
		}

		fmt.Println("List objects: ")
		for _, oi := range loi {
			fmt.Printf("\n")
			fmt.Println(FormatObjectInfo(oi))
		}

		return nil
	},
}

var putObjectCmd = &cli.Command{
	Name:  "putObject",
	Usage: "put object",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "bucket",
			Aliases: []string{"bn"},
			Usage:   "bucketName",
		},
		&cli.StringFlag{
			Name:    "object",
			Aliases: []string{"on"},
			Usage:   "objectName",
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

		poo := types.DefaultUploadOption()

		oi, err := napi.PutObject(cctx.Context, bucketName, objectName, pf, poo)
		if err != nil {
			return err
		}

		fmt.Println("Put object: ")
		fmt.Println(FormatObjectInfo(oi))

		return nil
	},
}

var headObjectCmd = &cli.Command{
	Name:  "headObject",
	Usage: "head object",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "bucket",
			Aliases: []string{"bn"},
			Usage:   "bucketName",
		},
		&cli.StringFlag{
			Name:    "object",
			Aliases: []string{"on"},
			Usage:   "objectName",
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

		fmt.Println("Head object: ")
		fmt.Println(FormatObjectInfo(oi))

		for i, part := range oi.Parts {
			fmt.Println("Part: ", i, part)
		}

		return nil
	},
}

var getObjectCmd = &cli.Command{
	Name:  "getObject",
	Usage: "get object",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "bucket",
			Aliases: []string{"bn"},
			Usage:   "bucketName",
		},
		&cli.StringFlag{
			Name:    "object",
			Aliases: []string{"on"},
			Usage:   "objectName",
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

		f, err := os.Create(p)
		if err != nil {
			return err
		}
		defer f.Close()

		doo := types.DefaultDownloadOption()

		data, err := napi.GetObject(cctx.Context, bucketName, objectName, doo)
		if err != nil {
			return err
		}

		f.Write(data)

		etag := md5.Sum(data)

		fmt.Println("get object: ", objectName, hex.EncodeToString(etag[:]))

		return nil
	},
}
