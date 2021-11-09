package lfs

import (
	"context"
	"crypto/cipher"
	"encoding/binary"
	"io"

	"github.com/memoio/go-mefs-v2/lib/crypto/aes"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/zeebo/blake3"
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

	if readStart > int64(object.Length) ||
		readStart+readLength > int64(object.Length) {
		return ErrObjectOptionsInvalid
	}

	if readLength <= 0 {
		readLength = int64(object.Length - uint64(readStart))
	}

	dp, ok := l.dps[bucket.BucketID]
	if !ok {
		ndp, err := l.newDataProcess(bucket.BucketID, &bucket.BucketOption)
		if err != nil {
			return err
		}
		dp = ndp
	}

	var aesDec cipher.BlockMode
	if object.Encryption == "aes" {
		tmpkey := make([]byte, 40)
		copy(tmpkey, dp.aesKey[:])
		binary.BigEndian.PutUint64(tmpkey[32:], object.ObjectID)
		hres := blake3.Sum256(tmpkey)

		tmpEnc, err := aes.ContructAesDec(hres[:])
		if err != nil {
			return err
		}
		aesDec = tmpEnc

		// read from begin if encrypts
		readLength += readStart
		readStart = 0
	}

	// read from each part
	accLen := uint64(0)
	rLen := uint64(0)
	for _, part := range object.Parts {
		if rLen >= uint64(readLength) {
			break
		}

		if accLen >= uint64(readStart+readLength) {
			break
		}

		if part.Length+accLen < uint64(readStart) {
			accLen += part.Length
			continue
		}

		partStart := part.Offset
		if uint64(readStart) > accLen {
			partStart += (uint64(readStart) - accLen)
		}

		partLength := part.Length
		if uint64(readStart+readLength) < accLen+part.Length {
			partLength = (uint64(readStart+readLength) - accLen)
		}

		err = l.download(ctx, dp, aesDec, bucket, object, int(partStart), int(partLength), writer)
		if err != nil {
			return err
		}
		rLen += partLength

		accLen += part.Length
	}
	return nil
}
