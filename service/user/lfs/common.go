package lfs

import (
	"github.com/minio/minio-go/v7/pkg/s3utils"
	"golang.org/x/xerrors"

	logging "github.com/memoio/go-mefs-v2/lib/log"
)

var logger = logging.Logger("lfs")

const (
	defaultWeighted = 1000
	maxBucket       = 256
	SlashSeparator  = "/"
)

var (
	ErrLfsServiceNotReady  = xerrors.New("lfs service is not ready, waiting for it")
	ErrLfsReadOnly         = xerrors.New("lfs service is read only")
	ErrResourceUnavailable = xerrors.New("resource unavailable, wait other lfs operations completed")
)

type MetaName string

func (x MetaName) LessThan(y interface{}) bool {
	yStr := y.(MetaName)
	return x < yStr
}

//检查文件名合法性
func checkBucketName(bucketName string) error {
	return s3utils.CheckValidBucketName(bucketName)
}

func checkObjectName(objectName string) error {
	return s3utils.CheckValidObjectName(objectName)
}
