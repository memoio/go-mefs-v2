package lfscmd

import (
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"fmt"
	"os"

	"github.com/ipfs/go-cid"
	"github.com/mitchellh/go-homedir"
	mh "github.com/multiformats/go-multihash"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/api/client"
	"github.com/memoio/go-mefs-v2/app/cmd"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/utils/etag"
)

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
		// get repo path from flag
		repoDir := cctx.String(cmd.FlagNodeRepo)
		// parse api ip:port from repo
		ip, headers, err := client.GetMemoClientInfo(repoDir)
		if err != nil {
			return err
		}

		// create user node from server, type: api.UserNodeStruct
		napi, closer, err := client.NewUserNode(cctx.Context, ip, headers)
		if err != nil {
			return err
		}
		defer closer()

		// get data from params
		bucketName := cctx.String("bucket")
		objectName := cctx.String("object")
		path := cctx.String("path")

		// get full path of home dir
		p, err := homedir.Expand(path)
		if err != nil {
			return err
		}

		// open repo dir
		pf, err := os.Open(p)
		if err != nil {
			return err
		}
		defer pf.Close()

		// create put object options
		poo := types.DefaultUploadOption()

		// execute putObject
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
			fmt.Printf("Part: %d, %s\n", i, FormatPartInfo(part))
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

		objInfo, err := napi.HeadObject(cctx.Context, bucketName, objectName)
		if err != nil {
			return err
		}

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

		var etagb []byte
		if len(objInfo.Etag) == md5.Size {
			etag16 := md5.Sum(data)
			etagb = etag16[:]
		} else {
			etag32 := sha256.Sum256(data)
			mhtag, err := mh.Encode(etag32[:], mh.SHA2_256)
			if err != nil {
				return err
			}

			cidEtag := cid.NewCidV1(cid.Raw, mhtag)
			etagb = cidEtag.Bytes()
		}

		gotEtag, err := etag.ToString(etagb)
		if err != nil {
			return err
		}

		origEtag, err := etag.ToString(objInfo.Etag)
		if err != nil {
			return err
		}

		if !bytes.Equal(etagb, objInfo.Etag) {
			return xerrors.Errorf("object content wrong, expect %s got %s", origEtag, gotEtag)
		}

		fmt.Printf("object %s (etag: %s) stored in file %s\n", objectName, gotEtag, p)

		return nil
	},
}

var delObjectCmd = &cli.Command{
	Name:  "delObject",
	Usage: "delete object",
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

		_, err = napi.DeleteObject(cctx.Context, bucketName, objectName)
		if err != nil {
			return err
		}

		fmt.Println("object ", objectName, " deleted")

		return nil
	},
}
