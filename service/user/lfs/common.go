package lfs

import (
	"errors"

	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/minio/minio-go/v6/pkg/s3utils"
)

var logger = logging.Logger("lfs")

const (
	defaultWeighted = 100
	MaxBucket       = 16
)

var (
	ErrEncode = errors.New("encode is wrong")

	ErrPolicy              = errors.New("policy is error")
	ErrLfsServiceNotReady  = errors.New("lfs service is not ready, please restart lfs")
	ErrLfsReadOnly         = errors.New("lfs service is read only")
	ErrLfsStarting         = errors.New("another lfs instance is starting")
	ErrUpload              = errors.New("upload fails")
	ErrResourceUnavailable = errors.New("resource unavailable, wait other option about lfs completed")
	ErrWrongParameters     = errors.New("wrong parameters")
	ErrCanceled            = errors.New("canceled")

	ErrNoProviders      = errors.New("there is no providers has the designated block")
	ErrNoKeepers        = errors.New("there is no keepers")
	ErrNoEnoughProvider = errors.New("no enough providers can be connected")
	ErrNoEnoughKeeper   = errors.New("no enough keepers can be connected")

	ErrBucketNotExist     = errors.New("bucket not exist")
	ErrBucketAlreadyExist = errors.New("bucket already exists")
	ErrBucketNotEmpty     = errors.New("bucket is not empty")
	ErrBucketNameInvalid  = errors.New("bucket name is invalid")
	ErrBucketTooMany      = errors.New("bucket is too many")

	ErrObjectNotExist       = errors.New("object not exist")
	ErrObjectIsNil          = errors.New("object is nil")
	ErrObjectAlreadyExist   = errors.New("object already exist")
	ErrObjectNameToolong    = errors.New("object name is too long")
	ErrObjectNameInvalid    = errors.New("object name is invalid")
	ErrObjectOptionsInvalid = errors.New("object option is invalid")
	ErrObjectIsDir          = errors.New("object is directory")
	ErrNoEnoughBlockUpload  = errors.New("block uploaded is not enough")
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
