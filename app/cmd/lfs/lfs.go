package lfscmd

import (
	"fmt"
	"time"

	"github.com/memoio/go-mefs-v2/api/client"
	"github.com/memoio/go-mefs-v2/app/cmd"
	"github.com/memoio/go-mefs-v2/lib/code"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/utils"
	"github.com/memoio/go-mefs-v2/lib/utils/etag"
	"github.com/mgutz/ansi"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

const (
	bucketNameKwd  = "bucketname"
	policyKwd      = "policy"
	dataCountKwd   = "datacount"
	parityCountKwd = "paritycount"
)

func FormatPolicy(policy uint32) string {
	if policy == code.RsPolicy {
		return "erasure code"
	} else if policy == code.MulPolicy {
		return "multi replica"

	}
	return "unknown"
}

func FormatBucketInfo(bucket types.BucketInfo) string {
	// get reliability with dc and pc
	reliability := ReliabilityLevel(bucket.DataCount, bucket.ParityCount)

	return fmt.Sprintf(
		`Name: %s
Bucket ID: %d
Policy: %s
Data Count: %d
Parity Count: %d
Reliability: %s
Confirmed: %t
Object Count: %d
Size: %s
Used Bytes: %s
Creation Time: %s
Modify Time: %s`,
		ansi.Color(bucket.Name, "green"),
		bucket.BucketID,
		FormatPolicy(bucket.Policy),
		bucket.DataCount,
		bucket.ParityCount,
		reliability,
		bucket.Confirmed,
		bucket.NextObjectID,
		utils.FormatBytes(int64(bucket.Length)),
		utils.FormatBytes(int64(bucket.UsedBytes)),
		time.Unix(int64(bucket.CTime), 0).Format(utils.SHOWTIME),
		time.Unix(int64(bucket.MTime), 0).Format(utils.SHOWTIME),
	)
}

func FormatObjectInfo(object types.ObjectInfo) string {
	setag, _ := etag.ToString(object.ETag)
	return fmt.Sprintf(
		`Name: %s
Bucket ID: %d
Object ID: %d
ETag: %s
Size: %s
UsedBytes: %s
Enc Method: %s
State: %s
Creation Time: %s
Modify Time: %s`,
		ansi.Color(object.Name, "green"),
		object.BucketID,
		object.ObjectID,
		setag,
		utils.FormatBytes(int64(object.Size)),
		utils.FormatBytes(int64(object.StoredBytes)),
		object.Encryption,
		object.State,
		time.Unix(int64(object.Time), 0).Format(utils.SHOWTIME),
		time.Unix(int64(object.Mtime), 0).Format(utils.SHOWTIME),
	)
}

func FormatPartInfo(pi *pb.ObjectPartInfo) string {
	setag, _ := etag.ToString(pi.ETag)
	return fmt.Sprintf("ObjectID: %d, Size: %d, CreationTime: %s, Offset: %d, DataBytes: %d, ETag: %s", pi.ObjectID, pi.Length, time.Unix(int64(pi.Time), 0).Format(utils.SHOWTIME), pi.Offset, pi.StoredBytes, setag)
}

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
		listObjectsCmd,
		delObjectCmd,
		downlaodObjectCmd,
		showStorageCmd,
	},
}

var createBucketCmd = &cli.Command{
	Name:  "createBucket",
	Usage: "create bucket",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "bucket",
			Aliases: []string{"bn"},
			Usage:   "bucketName",
		},
		&cli.UintFlag{
			Name:    policyKwd,
			Aliases: []string{"pl"},
			Usage:   "erasure code(1) or multi-replica(2)",
			Value:   code.RsPolicy,
		},
		&cli.UintFlag{
			Name:    dataCountKwd,
			Aliases: []string{"dc"},
			Usage:   "data count",
			Value:   3,
		},
		&cli.UintFlag{
			Name:    parityCountKwd,
			Aliases: []string{"pc"},
			Usage:   "parity count",
			Value:   2,
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
		if bucketName == "" {
			return xerrors.New("bucketname is nil")
		}

		opts := code.DefaultBucketOptions()
		opts.Policy = uint32(cctx.Uint(policyKwd))
		opts.DataCount = uint32(cctx.Uint(dataCountKwd))
		opts.ParityCount = uint32(cctx.Uint(parityCountKwd))

		bi, err := napi.CreateBucket(cctx.Context, bucketName, opts)
		if err != nil {
			return err
		}

		fmt.Println(FormatBucketInfo(bi))

		return nil
	},
}

var listBucketsCmd = &cli.Command{
	Name:  "listBuckets",
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

		fmt.Println("List buckets: ")
		for _, bi := range bs {
			fmt.Printf("-------\n")
			fmt.Println(FormatBucketInfo(bi))
		}

		return nil
	},
}

var headBucketCmd = &cli.Command{
	Name:  "headBucket",
	Usage: "head bucket info",
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

		bi, err := napi.HeadBucket(cctx.Context, bucketName)
		if err != nil {
			return err
		}

		fmt.Println("Head bucket: ")
		fmt.Println(FormatBucketInfo(bi))

		return nil
	},
}

var showStorageCmd = &cli.Command{
	Name:  "showStorage",
	Usage: "show storage info",
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

		ss, err := napi.ShowStorage(cctx.Context)
		if err != nil {
			return err
		}

		fmt.Println("Lfs has storage: ", utils.FormatBytes(int64(ss)))

		return nil
	},
}

// get security level from dataCount and parityCount
func ReliabilityLevel(dataCount uint32, parityCount uint32) string {
	reLevel := "Risky"

	// assume each node reliabilty is 0.9
	res := utils.CalReliabilty(int(dataCount+parityCount), int(dataCount), 0.9)
	if res > 0.9999 {
		reLevel = "High"
	} else if res > 0.99 {
		reLevel = "Medium"
	}

	return reLevel
}
