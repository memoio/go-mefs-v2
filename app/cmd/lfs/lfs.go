package lfscmd

import (
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/memoio/go-mefs-v2/api/client"
	"github.com/memoio/go-mefs-v2/app/cmd"
	"github.com/memoio/go-mefs-v2/lib/code"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/utils"
	"github.com/mgutz/ansi"
	"github.com/urfave/cli/v2"
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

func FormatBucketInfo(bucket *types.BucketInfo) string {
	// get reliability with dc and pc
	reliability := ReliabilityLevel(bucket.DataCount, bucket.ParityCount)

	return fmt.Sprintf(
		`Name: %s
Bucket ID: %d
Creation Time: %s
Modify Time: %s
Object Count: %d
Policy: %s
Data Count: %d
Parity Count: %d
Reliability: %s
Used Bytes: %s`,
		ansi.Color(bucket.Name, "green"),
		bucket.BucketID,
		time.Unix(int64(bucket.CTime), 0).Format(utils.SHOWTIME),
		time.Unix(int64(bucket.MTime), 0).Format(utils.SHOWTIME),
		bucket.NextObjectID,
		FormatPolicy(bucket.Policy),
		bucket.DataCount,
		bucket.ParityCount,
		reliability,
		utils.FormatBytes(int64(bucket.UsedBytes)),
	)
}

func FormatObjectInfo(object *types.ObjectInfo) string {
	return fmt.Sprintf(
		`Name: %s
Bucket ID: %d
Object ID: %d
Etag: %s
Creation Time: %s
Modify Time: %s
Size: %s
Enc Method: %s
State: %s`,
		ansi.Color(object.Name, "green"),
		object.BucketID,
		object.ObjectID,
		hex.EncodeToString(object.Etag),
		time.Unix(int64(object.Time), 0).Format(utils.SHOWTIME),
		time.Unix(int64(object.Mtime), 0).Format(utils.SHOWTIME),
		utils.FormatBytes(int64(object.Length)),
		object.Encryption,
		object.State,
	)
}

func FormatPartInfo(pi *pb.ObjectPartInfo) string {
	return fmt.Sprintf("ObjectID: %d, Size: %d, CreationTime: %s, Offset: %d, UsedBytes: %d, Etag: %s", pi.ObjectID, pi.Length, time.Unix(int64(pi.Time), 0).Format(utils.SHOWTIME), pi.Offset, pi.RawLength, hex.EncodeToString(pi.ETag))
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
			return errors.New("bucketname is nil")
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
			fmt.Printf("\n")
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

// get security level from dataCount and parityCount
func ReliabilityLevel(dataCount uint32, parityCount uint32) string {
	reLevel := ""

	// low security
	if dataCount > parityCount {
		reLevel = "LOW"
	}
	// medium security
	if dataCount == parityCount {
		reLevel = "MEDIUM"
	}
	// high security
	if dataCount < parityCount {
		reLevel = "HIGH"
	}

	return reLevel
}
