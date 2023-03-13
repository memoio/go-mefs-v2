package lfscmd

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/mitchellh/go-homedir"
	"github.com/schollz/progressbar/v3"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/api/client"
	"github.com/memoio/go-mefs-v2/app/cmd"
	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/etag"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/utils"
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
			Value: "aes2",
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

		if encFlag != "aes" && encFlag != "aes1" && encFlag != "aes2" && encFlag != "aes3" && encFlag != "none" {
			return xerrors.Errorf("support only 'none' or 'aes', 'aes1', 'aes2', 'aes3' method ")
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
		default:
			poo.UserDefined["etag"] = etagFlag
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
			Name:    "etag",
			Aliases: []string{"cid", "md5"},
			Usage:   "etag (cid or md5)",
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
		&cli.StringFlag{
			Name:  "decrypt",
			Usage: "decryption key",
		},
		&cli.StringFlag{
			Name:  "userID",
			Usage: "userID",
		},
		&cli.StringFlag{
			Name:  "bucketID",
			Usage: "bucketID",
		},
		&cli.StringFlag{
			Name:  "objectID",
			Usage: "objectID",
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
		etagName := cctx.String("etag")
		path := cctx.String("path")

		start := cctx.Int64("start")
		length := cctx.Int64("length")

		if etagName != "" {
			bucketName = ""
			objectName = etagName
			var sb strings.Builder
			sb.WriteString(etagName)
			sb.WriteString("/")
			sb.WriteString(cctx.String("userID"))
			sb.WriteString("/")
			sb.WriteString(cctx.String("bucketID"))
			sb.WriteString("/")
			sb.WriteString(cctx.String("objectID"))
			etagName = sb.String()
		} else {
			etagName = objectName
		}

		objInfo, err := napi.HeadObject(cctx.Context, bucketName, etagName)
		if err != nil {
			return err
		}

		fmt.Println(FormatObjectInfo(objInfo))

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
		if len(objInfo.ETag) != md5.Size {

			_, ecid, err := cid.CidFromBytes(objInfo.ETag)
			if err != nil {
				return err
			}

			c := &etag.Config{
				BlockSize: build.DefaultSegSize,
				HashType:  int(ecid.Prefix().MhType),
			}

			if objInfo.UserDefined != nil {
				etags := objInfo.UserDefined["etag"]
				if strings.HasPrefix(etags, "cid") {
					etagss := strings.Split(etags, "-")
					if len(etagss) > 1 {
						c.BlockSize = int(utils.HumanStringLoaded(etagss[1]))
						if c.BlockSize < 1024 {
							c.BlockSize = build.DefaultSegSize
						}
					}
				}
			}
			h = etag.NewTreeWithConfig(c)
		}

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

		doo := types.DownloadObjectOptions{
			UserDefined: make(map[string]string),
		}
		doo.UserDefined["userID"] = cctx.String("userID")
		doo.UserDefined["bucketID"] = cctx.String("bucketID")
		doo.UserDefined["objectID"] = cctx.String("objectID")
		doo.UserDefined["decrypt"] = cctx.String("decrypt")

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

			doo.Start = startOffset
			doo.Length = readLen

			data, err := napi.GetObject(cctx.Context, bucketName, objectName, doo)
			if err != nil {
				return err
			}

			bar.Add64(readLen)
			h.Write(data)
			f.Write(data)

			startOffset += readLen
			stepacc *= 2
		}

		bar.Finish()

		etagb := h.Sum(nil)
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
				fmt.Println("check your decryption key and etag method!")
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

var downloadCmd = &cli.Command{
	Name:  "download",
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
			Name:    "etag",
			Aliases: []string{"cid", "md5"},
			Usage:   "etag (cid or md5)",
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
		etagName := cctx.String("etag")

		form := url.Values{}
		form.Set("bucket", bucketName)
		form.Set("object", objectName)
		form.Set("etag", etagName)
		form.Set("start", strconv.FormatInt(cctx.Int64("start"), 10))
		form.Set("length", strconv.FormatInt(cctx.Int64("length"), 10))

		haddr := "http://" + addr + "/gateway/download"
		hreq, err := http.NewRequestWithContext(cctx.Context, "POST", haddr, strings.NewReader(form.Encode()))
		if err != nil {
			return err
		}

		hreq.Header = header.Clone()
		hreq.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		hreq.Header.Add("Content-Length", strconv.Itoa(len(form.Encode())))

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

		if etagName != "" {
			fmt.Printf("object: %s is stored in: %s\n", etagName, p)
		} else {
			fmt.Printf("object: %s is stored in: %s\n", objectName, p)
		}

		return nil
	},
}

var uploadCmd = &cli.Command{
	Name:  "upload",
	Usage: "upload object using rpc",
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
		addr, header, err := client.GetMemoClientInfo(repoDir)
		if err != nil {
			return err
		}

		p, err := homedir.Expand(cctx.String("path"))
		if err != nil {
			return err
		}

		p, err = filepath.Abs(p)
		if err != nil {
			return err
		}

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

		bucketName := cctx.String("bucket")
		objectName := cctx.String("object")

		haddr := "http://" + addr + "/gateway/upload/" + bucketName + "/" + objectName
		hreq, err := http.NewRequest("POST", haddr, &pr)
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

		res, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		if resp.StatusCode != 200 {
			return xerrors.Errorf("response: %s, msg: %s", resp.Status, res)
		}

		fmt.Printf("complete upload: %s to bucket: %s, object: %s\n", p, bucketName, objectName)

		oi := new(types.ObjectInfo)
		err = json.Unmarshal(res, oi)
		if err != nil {
			fmt.Println(err)
			return nil
		}

		fmt.Println(FormatObjectInfo(*oi))

		return nil
	},
}
