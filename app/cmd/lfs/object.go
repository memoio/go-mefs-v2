package lfscmd

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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
			Usage:   "bucket name, priority",
		},
		&cli.StringFlag{
			Name:  "marker",
			Usage: "key start from, marker should exist",
			Value: "",
		},
		&cli.StringFlag{
			Name:  "prefix",
			Usage: "prefix of objects",
			Value: "",
		},
		&cli.StringFlag{
			Name:  "delimiter",
			Usage: "delimiter to group keys: '/' or ''",
			Value: "",
		},
		&cli.IntFlag{
			Name:  "maxKeys",
			Usage: "number of objects in return",
			Value: types.MaxListKeys,
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

		loo := types.ListObjectsOptions{
			Prefix:    cctx.String("prefix"),
			Marker:    cctx.String("marker"),
			Delimiter: cctx.String("delimiter"),
			MaxKeys:   cctx.Int("maxKeys"),
		}

		loi, err := napi.ListObjects(cctx.Context, bucketName, loo)
		if err != nil {
			return err
		}

		fmt.Printf("List objects: maxKeys %d, prefix: %s, start from: %s\n", loo.MaxKeys, loo.Prefix, loo.Marker)

		if len(loi.Prefixes) > 0 {
			fmt.Printf("== directories ==\n")
			for _, pres := range loi.Prefixes {
				fmt.Printf("--------\n")
				fmt.Println(pres)
			}
			fmt.Printf("\n")
			fmt.Printf("==== files ====\n")
		}

		for _, oi := range loi.Objects {
			fmt.Printf("--------\n")
			fmt.Println(FormatObjectInfo(oi))
		}

		if loi.IsTruncated {
			fmt.Printf("--------\n")
			fmt.Println("marker is: ", loi.NextMarker)
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
			Usage: "etag method",
			Value: "md5",
		},
		&cli.StringFlag{
			Name:  "enc",
			Usage: "encryption method",
			Value: "aes",
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
		encFlag := cctx.String("enc")

		if encFlag != "aes" && encFlag != "none" {
			return xerrors.Errorf("support only 'none' or 'aes' method ")
		}

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

		fi, err := pf.Stat()
		if err != nil {
			return err
		}

		bar := progressbar.DefaultBytes(fi.Size(), "upload:")
		pr := progressbar.NewReader(pf, bar)

		// create put object options
		poo := types.DefaultUploadOption()

		switch etagFlag {
		case "cid":
			poo = types.CidUploadOption()
		}

		poo.UserDefined["encryption"] = encFlag

		// execute putObject
		oi, err := napi.PutObject(cctx.Context, bucketName, objectName, &pr, poo)
		if err != nil {
			return err
		}

		bar.Finish()

		fmt.Println("object: ")
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
			Name:  "cid",
			Usage: "cid name",
		},
		&cli.Int64Flag{
			Name:  "start",
			Usage: "start position",
			Value: 0,
		},
		&cli.Int64Flag{
			Name:  "length",
			Usage: "read length",
			Value: -1,
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
		cidName := cctx.String("cid")
		path := cctx.String("path")

		start := cctx.Int64("start")
		length := cctx.Int64("length")

		if cidName != "" {
			bucketName = ""
			objectName = cidName
		}

		objInfo, err := napi.HeadObject(cctx.Context, bucketName, objectName)
		if err != nil {
			return err
		}

		if objInfo.Size == 0 {
			return xerrors.Errorf("empty file")
		}

		if length == -1 {
			length = int64(objInfo.Size)
		}
		bar := progressbar.DefaultBytes(length, "download:")

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

		h := md5.New()
		tr := etag.NewTree()

		stepLen := int64(build.DefaultSegSize * 16)
		stepAccMax := 16
		if bucketName != "" {
			buInfo, err := napi.HeadBucket(cctx.Context, bucketName)
			if err != nil {
				return err
			}

			// around 128MB
			stepAccMax = 512 / int(buInfo.DataCount)
			stepLen = int64(build.DefaultSegSize * buInfo.DataCount)
		}

		startOffset := start
		end := start + length
		stepacc := 1
		for startOffset < end {
			if stepacc > stepAccMax {
				stepacc = stepAccMax
			}
			readLen := stepLen*int64(stepacc) - (startOffset % stepLen)
			if end-startOffset < readLen {
				readLen = end - startOffset
			}

			doo := types.DownloadObjectOptions{
				Start:  startOffset,
				Length: readLen,
			}

			data, err := napi.GetObject(cctx.Context, bucketName, objectName, doo)
			if err != nil {
				return err
			}

			bar.Add64(readLen)

			if len(objInfo.ETag) == md5.Size {
				h.Write(data)
			} else {
				for start := int64(0); start < readLen; {
					stepLen := int64(build.DefaultSegSize)
					if start+stepLen > readLen {
						stepLen = readLen - start
					}
					cid := etag.NewCidFromData(data[start : start+stepLen])

					tr.AddCid(cid, uint64(stepLen))

					start += stepLen
				}
			}

			f.Write(data)

			startOffset += readLen
			stepacc *= 2
		}

		bar.Finish()

		var etagb []byte
		if len(objInfo.ETag) == md5.Size {
			etagb = h.Sum(nil)
		} else {
			cidEtag := tr.Root()
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

		if start == 0 && length == int64(objInfo.Size) {
			if !bytes.Equal(etagb, objInfo.ETag) {
				return xerrors.Errorf("object content wrong, expect %s got %s", origEtag, gotEtag)
			}
		}

		fmt.Printf("object: %s (etag: %s) is stored in: %s\n", objectName, gotEtag, p)

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

		err = napi.DeleteObject(cctx.Context, bucketName, objectName)
		if err != nil {
			return err
		}

		fmt.Println("object ", objectName, " deleted")

		return nil
	},
}

var downlaodObjectCmd = &cli.Command{
	Name:  "downloadObject",
	Usage: "download object using rpc",
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
			Name:  "cid",
			Usage: "cid name",
		},
		&cli.Int64Flag{
			Name:  "start",
			Usage: "start position",
			Value: 0,
		},
		&cli.Int64Flag{
			Name:  "length",
			Usage: "read length",
			Value: -1,
		},
		&cli.StringFlag{
			Name:  "path",
			Usage: "stored path of file",
		},
	},
	Action: func(cctx *cli.Context) error {
		repoDir := cctx.String(cmd.FlagNodeRepo)
		addr, header, err := client.GetMemoClientInfo(repoDir)
		if err != nil {
			return err
		}

		cidName := cctx.String("cid")
		path := cctx.String("path")

		bar := progressbar.DefaultBytes(-1, "download:")

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

		bucketName := cctx.String("bucket")
		objectName := cctx.String("object")

		start := cctx.Int64("start")
		length := cctx.Int64("length")

		haddr := "http://" + addr + "/gateway/" + bucketName + "/" + objectName + "/" + strconv.FormatInt(start, 10) + "/" + strconv.FormatInt(length, 10)

		if cidName != "" {
			haddr = "http://" + addr + "/gateway/cid/" + cidName + "/" + strconv.FormatInt(start, 10) + "/" + strconv.FormatInt(length, 10)
		} else {
			cidName = bucketName + "/" + objectName
		}

		hreq, err := http.NewRequest("GET", haddr, nil)
		if err != nil {
			return err
		}

		hreq.Header = header.Clone()

		defaultHTTPClient := &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
					DualStack: true,
				}).DialContext,
				ForceAttemptHTTP2:     true,
				WriteBufferSize:       16 << 10, // 16KiB moving up from 4KiB default
				ReadBufferSize:        16 << 10, // 16KiB moving up from 4KiB default
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   100,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
				DisableCompression:    true,
			},
		}

		resp, err := defaultHTTPClient.Do(hreq)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return xerrors.Errorf("response: %s", resp.Status)
		}

		// read 1MB once
		readSize := build.DefaultSegSize * 4
		buf := make([]byte, readSize)

		breakFlag := false
		for !breakFlag {
			n, err := io.ReadAtLeast(resp.Body, buf[:readSize], readSize)
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				breakFlag = true
			} else if err != nil {
				return err
			}

			if n == 0 {
				log.Println("received zero length")
			}

			bar.Add(n)
			f.Write(buf[:n])
		}

		bar.Finish()

		fmt.Printf("object: %s is stored in: %s\n", cidName, p)

		return nil
	},
}
