package lfs

import (
	"github.com/minio/minio-go/v7/pkg/s3utils"
	"golang.org/x/xerrors"

	logging "github.com/memoio/go-mefs-v2/lib/log"
)

var logger = logging.Logger("lfs")

const (
	defaultWeighted = 100
	maxBucket       = 256
)

var (
	ErrEncode = xerrors.New("encode is wrong")

	ErrPolicy              = xerrors.New("policy is error")
	ErrLfsServiceNotReady  = xerrors.New("lfs service is not ready, waiting for it")
	ErrLfsReadOnly         = xerrors.New("lfs service is read only")
	ErrLfsStarting         = xerrors.New("another lfs instance is starting")
	ErrUpload              = xerrors.New("upload fails")
	ErrResourceUnavailable = xerrors.New("resource unavailable, wait other option about lfs completed")
	ErrWrongParameters     = xerrors.New("wrong parameters")
	ErrCanceled            = xerrors.New("canceled")

	ErrNoProviders      = xerrors.New("there is no providers has the designated block")
	ErrNoKeepers        = xerrors.New("there is no keepers")
	ErrNoEnoughProvider = xerrors.New("no enough providers can be connected")
	ErrNoEnoughKeeper   = xerrors.New("no enough keepers can be connected")

	ErrBucketNotExist     = xerrors.New("bucket not exist")
	ErrBucketAlreadyExist = xerrors.New("bucket already exists")
	ErrBucketNotEmpty     = xerrors.New("bucket is not empty")
	ErrBucketNameInvalid  = xerrors.New("bucket name is invalid")
	ErrBucketTooMany      = xerrors.New("bucket is too many")
	ErrBucketIsConfirm    = xerrors.New("bucket is confirming")

	ErrObjectNotExist       = xerrors.New("object not exist")
	ErrObjectIsNil          = xerrors.New("object is nil")
	ErrObjectAlreadyExist   = xerrors.New("object already exist")
	ErrObjectNameToolong    = xerrors.New("object name is too long")
	ErrObjectNameInvalid    = xerrors.New("object name is invalid")
	ErrObjectOptionsInvalid = xerrors.New("object option is invalid")
	ErrObjectIsDir          = xerrors.New("object is directory")
	ErrNoEnoughBlockUpload  = xerrors.New("block uploaded is not enough")
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
