package lfs

import (
	"context"

	"golang.org/x/sync/semaphore"

	"github.com/memoio/go-mefs-v2/api"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

type LfsService struct {
	api.IWallet
	api.IRole

	ds store.KVStore

	ctx        context.Context
	fsID       []byte // keyset的verifyKey的hash
	encryptKey []byte

	keyset pdpcommon.KeySet
	sb     *superBlock
	sw     *semaphore.Weighted
}

func New(ctx context.Context, ds store.KVStore) *LfsService {
	ls := &LfsService{
		ctx: ctx,
		ds:  ds,
		sw:  semaphore.NewWeighted(defaultWeighted),
	}

	return ls
}

func (l *LfsService) Start() error {
	return nil
}

func (l *LfsService) Stop() error {
	return nil
}

func (l *LfsService) Ready() bool {
	if l.sb == nil || l.sb.bucketNameToID == nil {
		return false
	}
	return l.sb.ready
}

func (l *LfsService) Writeable() bool {
	return l.sb.write
}

// ShowStorage show lfs used space without appointed bucket
func (l *LfsService) ShowStorage(ctx context.Context) (uint64, error) {
	ok := l.sw.TryAcquire(1)
	if !ok {
		return 0, ErrResourceUnavailable
	}
	defer l.sw.Release(1)

	if !l.Ready() { //只读不需要Online
		return 0, ErrLfsServiceNotReady
	}

	var storageSpace uint64
	for _, bucket := range l.sb.buckets {
		bucketStorage, err := l.ShowBucketStorage(ctx, bucket.Name)
		if err != nil {
			continue
		}
		storageSpace += uint64(bucketStorage)
	}

	return storageSpace, nil
}

// ShowBucketStorage show lfs used spaceBucket
func (l *LfsService) ShowBucketStorage(ctx context.Context, bucketName string) (uint64, error) {
	bucket, err := l.getBucketInfo(bucketName)
	if err != nil {
		return 0, err
	}

	bucket.RLock()
	defer bucket.RUnlock()
	var storageSpace uint64
	objectIter := bucket.objects.Iterator()
	for ; objectIter != nil; objectIter = objectIter.Next() {
		object := objectIter.Value.(*object)
		if object.deletion {
			continue
		}
		storageSpace += uint64(object.Length)
	}
	return storageSpace, nil
}
