package lfs

import (
	"context"
	"strings"

	"github.com/memoio/go-mefs-v2/lib/code"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"golang.org/x/xerrors"
)

func (l *LfsService) addBucket(bucketName string, opt *pb.BucketOption) (*types.BucketInfo, error) {
	l.sb.Lock()
	defer l.sb.Unlock()

	if _, ok := l.sb.bucketNameToID[bucketName]; ok {
		return nil, xerrors.Errorf("bucket %s already exists", bucketName)
	}

	bucketID := l.sb.NextBucketID

	bucket, err := l.createBucket(bucketID, bucketName, opt)
	if err != nil {
		return nil, err
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
		return nil, err
	}

	return &bucket.BucketInfo, nil
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

func (l *LfsService) CreateBucket(ctx context.Context, bucketName string, opt *pb.BucketOption) (*types.BucketInfo, error) {
	ok := l.sw.TryAcquire(1)
	if !ok {
		return nil, ErrResourceUnavailable
	}
	defer l.sw.Release(1)

	if !l.Writeable() {
		return nil, ErrLfsReadOnly
	}

	err := checkBucketName(bucketName)
	if err != nil {
		return nil, xerrors.Errorf("bucket name is invalid: %s", err)
	}

	if opt.DataCount == 0 || opt.ParityCount == 0 {
		return nil, xerrors.Errorf("data or parity count should not be zero")
	}

	if len(l.sb.buckets) >= int(maxBucket) {
		return nil, xerrors.Errorf("buckets are exceed %d", maxBucket)
	}

	switch opt.Policy {
	case code.MulPolicy:
		chunkCount := opt.DataCount + opt.ParityCount
		opt.DataCount = 1
		opt.ParityCount = chunkCount - 1
	case code.RsPolicy:
		if opt.DataCount == 1 {
			opt.Policy = code.MulPolicy
		}
	default:
		return nil, xerrors.Errorf("policy %d is not supported", opt.Policy)
	}

	return l.addBucket(bucketName, opt)
}

func (l *LfsService) DeleteBucket(ctx context.Context, bucketName string) (*types.BucketInfo, error) {
	ok := l.sw.TryAcquire(1)
	if !ok {
		return nil, ErrResourceUnavailable
	}
	defer l.sw.Release(1)

	if !l.Writeable() {
		return nil, ErrLfsReadOnly
	}

	l.sb.RLock()
	defer l.sb.RUnlock()

	bucket, err := l.getBucketInfo(bucketName)
	if err != nil {
		return nil, err
	}

	if bucket.BucketID >= l.sb.bucketVerify {
		return nil, xerrors.Errorf("bucket %d is confirming", bucket.BucketID)
	}

	bucket.Lock()
	defer bucket.Unlock()

	bucket.BucketInfo.Deletion = true
	bucket.dirty = true

	err = bucket.Save(l.userID, l.ds)
	if err != nil {
		return nil, err
	}

	return &bucket.BucketInfo, nil
}

func (l *LfsService) HeadBucket(ctx context.Context, bucketName string) (*types.BucketInfo, error) {
	ok := l.sw.TryAcquire(1)
	if !ok {
		return nil, ErrResourceUnavailable
	}
	defer l.sw.Release(1)

	// get bucket from bucket name
	bucket, err := l.getBucketInfo(bucketName)
	if err != nil {
		return nil, err
	}

	if bucket.BucketID < l.sb.bucketVerify {
		bucket.Confirmed = true
	}

	return &bucket.BucketInfo, nil
}

func (l *LfsService) ListBuckets(ctx context.Context, prefix string) ([]*types.BucketInfo, error) {
	ok := l.sw.TryAcquire(1)
	if !ok {
		return nil, ErrResourceUnavailable
	}
	defer l.sw.Release(1)

	l.sb.RLock()
	defer l.sb.RUnlock()

	buckets := make([]*types.BucketInfo, 0, len(l.sb.buckets))
	for _, b := range l.sb.buckets {
		if !b.BucketInfo.Deletion {
			if strings.HasPrefix(b.GetName(), prefix) {
				buckets = append(buckets, &b.BucketInfo)
			}
		}
	}

	return buckets, nil
}
