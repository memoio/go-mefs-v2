package lfs

import (
	"context"
	"sync"

	"golang.org/x/sync/semaphore"

	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/zeebo/blake3"
)

type LfsService struct {
	sync.RWMutex

	ctx      context.Context
	keyset   pdpcommon.KeySet
	ds       store.KVStore
	segStore segment.SegmentStore

	userID     uint64
	fsID       []byte // keyset的verifyKey的hash
	encryptKey []byte

	sb *superBlock

	dps map[uint64]*dataProcess

	sw *semaphore.Weighted // manage resource
}

func New(ctx context.Context, userID uint64, keyset pdpcommon.KeySet, ds store.KVStore, ss segment.SegmentStore) (*LfsService, error) {
	ls := &LfsService{
		ctx: ctx,

		userID: userID,
		fsID:   make([]byte, 20),

		ds:       ds,
		segStore: ss,
		keyset:   keyset,

		sb:  newSuperBlock(),
		dps: make(map[uint64]*dataProcess),
		sw:  semaphore.NewWeighted(defaultWeighted),
	}

	vk := keyset.VerifyKey().Serialize()
	fsIDBytes := blake3.Sum256(vk)
	copy(ls.fsID, fsIDBytes[:20])

	// load lfs info first
	err := ls.Load()
	if err != nil {
		return nil, err
	}

	go ls.persistMeta()

	return ls, nil
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
