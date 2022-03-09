package lfs

import (
	"bytes"
	"context"

	"github.com/memoio/go-mefs-v2/lib/types"
)

// read at most one stripe
func (l *LfsService) GetObject(ctx context.Context, bucketName, objectName string, opts *types.DownloadObjectOptions) ([]byte, error) {
	ok := l.sw.TryAcquire(10)
	if !ok {
		return nil, ErrResourceUnavailable
	}
	defer l.sw.Release(10)

	if !l.Ready() {
		return nil, ErrLfsServiceNotReady
	}

	bucket, err := l.getBucket(bucketName)
	if err != nil {
		return nil, err
	}

	object, err := l.getObjectInfo(bucket, objectName)
	if err != nil {
		return nil, err
	}

	object.RLock()
	defer object.RUnlock()

	if object.deletion {
		return nil, ErrObjectNotExist
	}

	if object.Length == 0 {
		return nil, ErrObjectIsNil
	}

	readStart := opts.Start
	readLength := opts.Length

	if readStart > int64(object.Length) ||
		readStart+readLength > int64(object.Length) {
		return nil, ErrObjectOptionsInvalid
	}

	dp, ok := l.dps[bucket.BucketID]
	if !ok {
		ndp, err := l.newDataProcess(bucket.BucketID, &bucket.BucketOption)
		if err != nil {
			return nil, err
		}
		dp = ndp
	}

	if readLength <= 0 {
		readLength = int64(object.Length - uint64(readStart))
	}

	buf := new(bytes.Buffer)

	// read from each part
	accLen := uint64(0)
	rLen := uint64(0)
	for _, part := range object.Parts {
		logger.Debug("part: ", part.Offset, part.Length, part.RawLength)

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

		err = l.download(ctx, dp, bucket, object, int(partStart), int(partLength), buf)
		if err != nil {
			return nil, err
		}
		rLen += partLength

		accLen += part.Length
	}
	return buf.Bytes(), nil
}
