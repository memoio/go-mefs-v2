package lfscmd

import (
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mitchellh/go-homedir"
	"github.com/schollz/progressbar/v3"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/api/client"
	"github.com/memoio/go-mefs-v2/app/cmd"
	"github.com/memoio/go-mefs-v2/build"
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
		&cli.StringFlag{
			Name:  "etag",
			Usage: "etag medthd",
			Value: "md5",
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
		etagFlag := cctx.String("etag")

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

		switch etagFlag {
		case "cid":
			poo = types.CidUploadOption()
		}

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
		&cli.BoolFlag{
			Name:  "all",
			Usage: "show all information",
			Value: false,
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

		if cctx.Bool("all") {
			for i, part := range oi.Parts {
				fmt.Printf("Part: %d, %s\n", i, FormatPartInfo(part))
			}
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
		&cli.BoolFlag{
			Name:  "local",
			Usage: "local file path",
			Value: false,
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
		localFlag := cctx.Bool("local")

		objInfo, err := napi.HeadObject(cctx.Context, bucketName, objectName)
		if err != nil {
			return err
		}

		if objInfo.Size == 0 {
			return xerrors.Errorf("empty file")
		}

		p, err := homedir.Expand(path)
		if err != nil {
			return err
		}

		p, err = filepath.Abs(p)
		if err != nil {
			return err
		}

		f, err := os.Create(p)
		if err != nil {
			return err
		}
		defer f.Close()

		if localFlag {
			doo := &types.DownloadObjectOptions{
				UserDefined: make(map[string]string),
				Start:       0,
				Length:      -1,
			}

			doo.UserDefined["local"] = p
		}

		h := md5.New()
		if len(objInfo.ETag) != md5.Size {
			h = sha256.New()
		}

		// around 64MB
		buInfo, err := napi.HeadBucket(cctx.Context, bucketName)
		if err != nil {
			return err
		}

		bar := progressbar.NewOptions64(
			int64(objInfo.Size),
			progressbar.OptionSetDescription("download:"),
			progressbar.OptionEnableColorCodes(true),
			progressbar.OptionSetWidth(10),
			progressbar.OptionThrottle(65*time.Millisecond),
			progressbar.OptionShowCount(),
			progressbar.OptionShowBytes(true),
			progressbar.OptionOnCompletion(func() {
				fmt.Printf("\n")
			}),
			progressbar.OptionSetTheme(progressbar.Theme{
				Saucer:        "[green]=[reset]",
				SaucerHead:    "[green]>[reset]",
				SaucerPadding: " ",
				BarStart:      "[",
				BarEnd:        "]",
			}),
			progressbar.OptionSpinnerType(14),
			progressbar.OptionFullWidth(),
		)

		stripeCnt := 4 * 64 / buInfo.DataCount
		stepLen := int64(build.DefaultSegSize * stripeCnt * buInfo.DataCount)
		start := int64(0)
		oSize := int64(objInfo.Size)
		for start < oSize {
			readLen := stepLen
			if oSize-start < stepLen {
				readLen = oSize - start
			}

			doo := &types.DownloadObjectOptions{
				Start:  start,
				Length: readLen,
			}

			data, err := napi.GetObject(cctx.Context, bucketName, objectName, doo)
			if err != nil {
				return err
			}

			bar.Add(int(readLen))

			h.Write(data)
			f.Write(data)

			start += readLen
		}

		var etagb []byte
		if len(objInfo.ETag) == md5.Size {
			etagb = h.Sum(nil)
		} else {
			cidEtag := etag.NewCid(h.Sum(nil))
			etagb = cidEtag.Bytes()
		}

		gotEtag, err := etag.ToString(etagb)
		if err != nil {
			return err
		}

		origEtag, err := etag.ToString(objInfo.ETag)
		if err != nil {
			return err
		}

		if !bytes.Equal(etagb, objInfo.ETag) {
			return xerrors.Errorf("object content wrong, expect %s got %s", origEtag, gotEtag)
		}

		fmt.Printf("object: %s (etag: %s) is stored in file %s\n", objectName, gotEtag, p)

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
