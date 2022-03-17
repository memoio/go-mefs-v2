package lfs

import (
	"bytes"
	"context"

	"github.com/memoio/go-mefs-v2/lib/types"

	"github.com/shirou/gopsutil/v3/mem"
	"golang.org/x/xerrors"
)

// read at most one stripe
func (l *LfsService) GetObject(ctx context.Context, bucketName, objectName string, opts *types.DownloadObjectOptions) ([]byte, error) {
	ok := l.sw.TryAcquire(2)
	if !ok {
		return nil, ErrResourceUnavailable
	}
	defer l.sw.Release(2)

	if !l.Ready() {
		return nil, ErrLfsServiceNotReady
	}

	// 1GB?
	if opts.Length > 1024*1024*1024 {
		v, err := mem.VirtualMemory()
		if err != nil {
			return nil, xerrors.Errorf("size is too large, consume too much memory")
		}
		if v.Available*10 < uint64(opts.Length)*12 {
			return nil, xerrors.Errorf("size is too large, memory is not enough")
		}
	}

	bucket, err := l.getBucketInfo(bucketName)
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

	if object.Size == 0 {
		return nil, ErrObjectIsNil
	}

	readStart := opts.Start
	readLength := opts.Length

	if readStart > int64(object.Size) ||
		readStart+readLength > int64(object.Size) {
		return nil, xerrors.Errorf("out of object size %d", object.Size)
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
		readLength = int64(object.Size - uint64(readStart))
	}

	buf := new(bytes.Buffer)

	// read from each part
	accLen := uint64(0) // sum of part length
	rLen := uint64(0)   // have read ok
	for _, part := range object.Parts {
		logger.Debug("part: ", part.Offset, part.StoredBytes, part.Length)

		// forward to part
		if accLen+part.Length <= uint64(readStart) {
			accLen += part.Length
			continue
		}

		partStart := part.Offset
		partLength := part.Length

		if uint64(readStart) > accLen {
			// move forward
			partStart += (uint64(readStart) - accLen)
			// sub head
			partLength -= (uint64(readStart) - accLen)
		}

		if uint64(readStart+readLength) < accLen+part.Length {
			// sub end
			partLength -= (accLen + part.Length - uint64(readStart+readLength))
		}

		err = l.download(ctx, dp, bucket, object, int(partStart), int(partLength), buf)
		if err != nil {
			return buf.Bytes(), err
		}
		rLen += partLength
		accLen += part.Length

		// read finish
		if rLen >= uint64(readLength) {
			break
		}

		// read to end
		if accLen >= uint64(readStart+readLength) {
			break
		}
	}
	return buf.Bytes(), nil
}
