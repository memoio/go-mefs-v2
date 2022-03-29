package types

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/memoio/go-mefs-v2/lib/code"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/utils"
	"github.com/mgutz/ansi"
)

const (
	MaxListKeys = 1000
)

type LfsInfo struct {
	Status bool
	Bucket uint64
	Used   uint64
}

type BucketInfo struct {
	pb.BucketOption
	pb.BucketInfo
	Confirmed bool `json:"Confirmed"`
}

func (bi BucketInfo) String() string {
	switch bi.Policy {
	case code.RsPolicy:
		return fmt.Sprintf("Name: %s, BucketID: %d, CreationTime: %s, ModifyTime: %s, ObjectCount: %d, Policy: erasure code, DataCount: %d, ParityCount: %d, UsedBytes: %s",
			ansi.Color(bi.Name, "green"),
			bi.BucketID,
			time.Unix(int64(bi.CTime), 0).Format(utils.SHOWTIME),
			time.Unix(int64(bi.MTime), 0).Format(utils.SHOWTIME),
			bi.NextObjectID,
			bi.DataCount,
			bi.ParityCount,
			utils.FormatBytes(int64(bi.UsedBytes)),
		)
	case code.MulPolicy:
		return fmt.Sprintf("Name: %s, BucketID: %d, CreationTime: %s, ModifyTime: %s, ObjectCount: %d, Policy: %d replicas, UsedBytes: %s",
			ansi.Color(bi.Name, "green"),
			bi.BucketID,
			time.Unix(int64(bi.CTime), 0).Format(utils.SHOWTIME),
			time.Unix(int64(bi.MTime), 0).Format(utils.SHOWTIME),
			bi.NextObjectID,
			bi.DataCount+bi.ParityCount,
			utils.FormatBytes(int64(bi.UsedBytes)),
		)
	default:
		return "unknown policy"
	}
}

type ObjectInfo struct {
	pb.ObjectInfo
	Parts       []*pb.ObjectPartInfo `json:"Parts"`
	Size        uint64               `json:"Size"`        // file size(sum of part.RawLength)
	StoredBytes uint64               `json:"StoredBytes"` // stored size(sum of part.Length)
	Mtime       int64                `json:"Mtime"`
	State       string               `json:"State"`
	ETag        []byte               `json:"MD5"`
}

func (oi ObjectInfo) String() string {
	return fmt.Sprintf("Name: %s, BucketID: %d, ObjectID: %d, ETag: %s, CreationTime: %s, ModifyTime: %s, Size: %s, EncMethod: %s, State: %s",
		ansi.Color(oi.Name, "green"),
		oi.BucketID,
		oi.ObjectID,
		hex.EncodeToString(oi.ETag),
		time.Unix(int64(oi.Time), 0).Format(utils.SHOWTIME),
		time.Unix(int64(oi.Mtime), 0).Format(utils.SHOWTIME),
		utils.FormatBytes(int64(oi.Size)),
		oi.Encryption,
		oi.State,
	)
}

type DownloadObjectOptions struct {
	UserDefined   map[string]string
	Start, Length int64
}

func DefaultDownloadOption() DownloadObjectOptions {
	return DownloadObjectOptions{
		Start:  0,
		Length: -1,
	}
}

type ListObjectsOptions struct {
	Prefix, Marker, Delimiter string
	MaxKeys                   int
}

func DefaultListOption() *ListObjectsOptions {
	return &ListObjectsOptions{
		MaxKeys: MaxListKeys,
	}
}

// similar to minio/s3
type ListObjectsInfo struct {
	IsTruncated bool

	NextMarker string

	Objects []ObjectInfo

	Prefixes []string
}

type PutObjectOptions struct {
	UserDefined map[string]string
}

func DefaultUploadOption() PutObjectOptions {
	poo := PutObjectOptions{
		UserDefined: make(map[string]string),
	}

	poo.UserDefined["encryption"] = "aes"
	poo.UserDefined["etag"] = "md5"

	return poo
}

func CidUploadOption() PutObjectOptions {
	poo := PutObjectOptions{
		UserDefined: make(map[string]string),
	}

	poo.UserDefined["encryption"] = "aes"
	poo.UserDefined["etag"] = "cid"
	return poo
}
