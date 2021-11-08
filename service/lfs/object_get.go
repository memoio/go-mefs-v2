package lfs

import (
	"context"
	"io"
	"time"

	"github.com/memoio/go-mefs-v2/lib/code"
	"github.com/memoio/go-mefs-v2/lib/crypto/aes"
	"github.com/memoio/go-mefs-v2/lib/types"
)

func (l *LfsService) GetObject(ctx context.Context, bucketName, objectName string, writer io.Writer, completeFuncs []types.CompleteFunc, opts types.DownloadObjectOptions) error {
	ok := l.sw.TryAcquire(10)
	if !ok {
		return ErrResourceUnavailable
	}
	defer l.sw.Release(10)

	if !l.Ready() {
		return ErrLfsServiceNotReady
	}

	bucket, err := l.getBucketInfo(bucketName)
	if err != nil {
		return err
	}

	object, err := l.getObjectInfo(bucket, objectName)
	if err != nil {
		return err
	}

	object.RLock()
	defer object.RUnlock()

	if object.deletion {
		return ErrObjectNotExist
	}

	if object.Length == 0 {
		return ErrObjectIsNil
	}

	readStart := opts.Start
	readLength := opts.Length

	if readLength <= 0 {
		readLength = int64(object.Length - uint64(readStart))
	}

	if readStart > int64(object.Length) ||
		readStart+readLength > int64(object.Length) {
		return ErrObjectOptionsInvalid
	}

	decoder, err := code.NewDataCoderWithBopts(l.keyset, &bucket.BucketOption)
	if err != nil {
		return err
	}

	dl := &downloadInstance{
		fsID:      l.fsID,
		encrypt:   object.Encryption,
		bucket:    bucket,
		decoder:   decoder,
		startTime: time.Now(),
		writer:    writer,
	}

	// default AES
	if dl.encrypt == "aes" {
		dl.sKey = aes.CreateAesKey([]byte(l.encryptKey), l.fsID, int64(bucket.BucketID), int64(object.ObjectID))
	}

	cumuLength := uint64(0)
	for _, part := range object.Parts {
		if cumuLength >= uint64(readStart+readLength) {
			break
		}

		if part.Length+cumuLength < uint64(readStart) {
			cumuLength += part.Length
			continue
		}

		var partStart = int64(0)
		var partLength = int64(0)

		if readStart > int64(cumuLength) {
			partStart = int64(part.Offset) + readStart - int64(cumuLength)
		} else {
			partStart = int64(part.Offset)
		}

		partLength = readLength + readStart - int64(cumuLength)

		dl.start = partStart
		dl.length = partLength
		dl.sizeReceived = 0

		err = dl.Start(ctx)
		if err != nil {
			return err
		}

		cumuLength += part.Length
	}
	return nil
}

type downloadInstance struct {
	start        int64 //在Bucket里起始的字节起始
	length       int64
	sizeReceived int64
	sKey         [32]byte
	fsID         []byte
	encrypt      string
	bucket       *bucket
	decoder      *code.DataCoder //用于解码数据
	startTime    time.Time
	writer       io.Writer
}

func (dl *downloadInstance) Start(ctx context.Context) error {
	return nil
}
