package types

import "github.com/memoio/go-mefs-v2/lib/pb"

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

type ObjectInfo struct {
	pb.ObjectInfo
	Parts  []*pb.ObjectPartInfo `json:"Parts"`
	Length uint64               `json:"Length"`
	Mtime  int64                `json:"Mtime"`
	State  string               `json:"State"`
	Etag   []byte               `json:"MD5"`
}

type DownloadObjectOptions struct {
	Start, Length int64
}

func DefaultDownloadOption() *DownloadObjectOptions {
	return &DownloadObjectOptions{
		Start:  0,
		Length: -1,
	}
}

type ListObjectsOptions struct {
	Prefix, Marker, Delimiter string
	MaxKeys                   int
	Recursive                 bool
}

func DefaultListOption() *ListObjectsOptions {
	return &ListObjectsOptions{
		MaxKeys:   MaxListKeys,
		Recursive: true,
	}
}

type PutObjectOptions struct {
	UserDefined map[string]string
}

func DefaultUploadOption() *PutObjectOptions {
	return &PutObjectOptions{
		UserDefined: make(map[string]string),
	}
}
