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

	// 512MB?
	if opts.Length > 512*1024*1024 {
		v, err := mem.VirtualMemory()
		if err != nil {
			return nil, xerrors.Errorf("size is too large, consume too much memory")
		}
		if v.Available*10 < uint64(opts.Length)*12 {
			return nil, xerrors.Errorf("size is too large, memory is not enough")
		}
	}

	if bucketName == "" && objectName == "" {
		return l.getObjectByCID(ctx, opts)
	}

	bucket, err := l.getBucketInfo(bucketName)
	if err != nil {
		return nil, err
	}

	if bucket.BucketInfo.Deletion {
		return nil, xerrors.Errorf("bucket %d is deleted", bucket.BucketID)
	}

	object, err := l.getObjectInfo(bucket, objectName)
	if err != nil {
		return nil, err
	}

	return l.downloadObject(ctx, bucket, object, opts)
}

func (l *LfsService) downloadObject(ctx context.Context, bucket *bucket, object *object, opts *types.DownloadObjectOptions) ([]byte, error) {
	object.RLock()
	defer object.RUnlock()

	if object.deletion {
		return nil, xerrors.Errorf("object %s is deleted", object.Name)
	}

	if object.Size == 0 {
		return nil, xerrors.New("object is empty")
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

	// length is zero
	if readLength == 0 {
		return buf.Bytes(), nil
	}

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

		err := l.download(ctx, dp, bucket, object, int(partStart), int(partLength), buf)
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

func (l *LfsService) getObjectByCID(ctx context.Context, opts *types.DownloadObjectOptions) ([]byte, error) {
	if opts.UserDefined == nil {
		return nil, xerrors.Errorf("empty cid name")
	}
	cidName, ok := opts.UserDefined["cid"]
	if !ok {
		return nil, xerrors.Errorf("cid name is not set")
	}

	od, ok := l.sb.cids[cidName]
	if !ok {
		return nil, xerrors.Errorf("file not exist")
	}

	if len(l.sb.buckets) < int(od.bucketID) {
		return nil, xerrors.Errorf("bucket %d not exist", od.bucketID)
	}

	bucket := l.sb.buckets[od.bucketID]
	if bucket.BucketInfo.Deletion {
		return nil, xerrors.Errorf("bucket %d is deleted", od.bucketID)
	}

	object, ok := bucket.objects[od.objectID]
	if !ok {
		return nil, xerrors.Errorf("object %d not exist", od.objectID)
	}

	return l.downloadObject(ctx, bucket, object, opts)
}
