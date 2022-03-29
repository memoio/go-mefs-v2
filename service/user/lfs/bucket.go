package lfs

import (
	"context"
	"strings"

	"github.com/memoio/go-mefs-v2/lib/code"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"golang.org/x/xerrors"
)

func (l *LfsService) addBucket(bucketName string, opts pb.BucketOption) (types.BucketInfo, error) {
	bi := types.BucketInfo{}
	l.sb.Lock()
	defer l.sb.Unlock()

	if _, ok := l.sb.bucketNameToID[bucketName]; ok {
		return bi, xerrors.Errorf("bucket %s already exists", bucketName)
	}

	bucketID := l.sb.NextBucketID

	bucket, err := l.createBucket(bucketID, bucketName, opts)
	if err != nil {
		return bi, err
	}

	bucket.Lock()
	defer bucket.Unlock()

	//将此Bucket信息添加到LFS中
	l.sb.bucketNameToID[bucketName] = bucketID
	l.sb.buckets = append(l.sb.buckets, bucket)
	l.sb.NextBucketID++
	l.sb.dirty = true

	err = l.sb.Save(l.userID, l.ds)
	if err != nil {
		return bi, err
	}

	return bucket.BucketInfo, nil
}

// get bucket with bucket name
func (l *LfsService) getBucketInfo(bucketName string) (*bucket, error) {
	err := checkBucketName(bucketName)
	if err != nil {
		return nil, xerrors.Errorf("bucket name is invalid: %s", err)
	}

	// get bucket ID
	bucketID, ok := l.sb.bucketNameToID[bucketName]
	if !ok {
		return nil, xerrors.Errorf("bucket %s not exist", bucketName)
	}

	if len(l.sb.buckets) < int(bucketID) {
		return nil, xerrors.Errorf("bucket %d not exist", bucketID)
	}

	// get bucket with ID
	bucket := l.sb.buckets[bucketID]
	if bucket.BucketInfo.Deletion {
		return nil, xerrors.Errorf("bucket %s is deleted", bucketName)
	}

	return bucket, nil
}

func (l *LfsService) CreateBucket(ctx context.Context, bucketName string, opts pb.BucketOption) (types.BucketInfo, error) {
	bi := types.BucketInfo{}
	ok := l.sw.TryAcquire(1)
	if !ok {
		return bi, ErrResourceUnavailable
	}
	defer l.sw.Release(1)

	if !l.Writeable() {
		return bi, ErrLfsReadOnly
	}

	err := checkBucketName(bucketName)
	if err != nil {
		return bi, xerrors.Errorf("bucket name is invalid: %s", err)
	}

	if opts.DataCount == 0 || opts.ParityCount == 0 {
		return bi, xerrors.Errorf("data or parity count should not be zero")
	}

	if len(l.sb.buckets) >= int(maxBucket) {
		return bi, xerrors.Errorf("buckets are exceed %d", maxBucket)
	}

	switch opts.Policy {
	case code.MulPolicy:
		chunkCount := opts.DataCount + opts.ParityCount
		opts.DataCount = 1
		opts.ParityCount = chunkCount - 1
	case code.RsPolicy:
		if opts.DataCount == 1 {
			opts.Policy = code.MulPolicy
		}
	default:
		return bi, xerrors.Errorf("policy %d is not supported", opts.Policy)
	}

	return l.addBucket(bucketName, opts)
}

func (l *LfsService) DeleteBucket(ctx context.Context, bucketName string) error {
	ok := l.sw.TryAcquire(1)
	if !ok {
		return ErrResourceUnavailable
	}
	defer l.sw.Release(1)

	if !l.Writeable() {
		return ErrLfsReadOnly
	}

	bucket, err := l.getBucketInfo(bucketName)
	if err != nil {
		return err
	}

	if bucket.BucketID >= l.sb.bucketVerify {
		return xerrors.Errorf("bucket %d is confirming", bucket.BucketID)
	}

	// remove data process
	l.Lock()
	delete(l.dps, bucket.BucketID)
	l.Unlock()

	bucket.Lock()
	defer bucket.Unlock()

	bucket.BucketInfo.Deletion = true
	bucket.dirty = true

	err = bucket.Save(l.userID, l.ds)
	if err != nil {
		return err
	}

	return nil
}

func (l *LfsService) HeadBucket(ctx context.Context, bucketName string) (types.BucketInfo, error) {
	bi := types.BucketInfo{}
	ok := l.sw.TryAcquire(1)
	if !ok {
		return bi, ErrResourceUnavailable
	}
	defer l.sw.Release(1)

	// get bucket from bucket name
	bucket, err := l.getBucketInfo(bucketName)
	if err != nil {
		return bi, err
	}

	if bucket.BucketID < l.sb.bucketVerify {
		bucket.Confirmed = true
	}

	return bucket.BucketInfo, nil
}

func (l *LfsService) ListBuckets(ctx context.Context, prefix string) ([]types.BucketInfo, error) {
	ok := l.sw.TryAcquire(1)
	if !ok {
		return nil, ErrResourceUnavailable
	}
	defer l.sw.Release(1)

	l.sb.RLock()
	defer l.sb.RUnlock()

	buckets := make([]types.BucketInfo, 0, len(l.sb.buckets))
	for _, b := range l.sb.buckets {
		if !b.BucketInfo.Deletion {
			if strings.HasPrefix(b.GetName(), prefix) {
				buckets = append(buckets, b.BucketInfo)
			}
		}
	}

	return buckets, nil
}
