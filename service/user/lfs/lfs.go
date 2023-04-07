package lfs

import (
	"context"
	"math/big"
	"os"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"

	lru "github.com/hashicorp/golang-lru"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	uorder "github.com/memoio/go-mefs-v2/service/user/order"
)

type LfsService struct {
	sync.RWMutex

	*uorder.OrderMgr

	ctx    context.Context
	keyset pdpcommon.KeySet
	ds     store.KVStore

	needPay *big.Int
	bal     *big.Int

	userID     uint64
	fsID       []byte // keyset的verifyKey的hash
	encryptKey []byte

	sb *superBlock

	dps map[uint64]*dataProcess

	sw *semaphore.Weighted // manage resource

	bucketChan      chan uint64
	bucketReadyChan chan uint64
	readyChan       chan struct{}
	msgChan         chan *tx.Message

	users    map[uint64]*ghost
	tagCache *lru.ARCCache
}

func New(ctx context.Context, userID uint64, encryptKey []byte, keyset pdpcommon.KeySet, ds store.KVStore, ss segment.SegmentStore, OrderMgr *uorder.OrderMgr) (*LfsService, error) {
	wt := defaultWeighted
	wts := os.Getenv("MEFS_LFS_PARALLEL")
	if wts != "" {
		wtv, err := strconv.Atoi(wts)
		if err == nil && wtv > 100 {
			wt = wtv
		}
	}

	tCache, err := lru.NewARC(1024 * 1024)
	if err != nil {
		return nil, err
	}

	ls := &LfsService{
		ctx: ctx,

		userID: userID,
		fsID:   make([]byte, 20),

		OrderMgr:   OrderMgr,
		ds:         ds,
		encryptKey: encryptKey,
		keyset:     keyset,

		needPay: new(big.Int),
		bal:     new(big.Int),

		sb:  newSuperBlock(),
		dps: make(map[uint64]*dataProcess),
		sw:  semaphore.NewWeighted(int64(wt)),

		readyChan:       make(chan struct{}, 1),
		bucketReadyChan: make(chan uint64),
		bucketChan:      make(chan uint64),
		msgChan:         make(chan *tx.Message, 128),

		users:    make(map[uint64]*ghost),
		tagCache: tCache,
	}

	ls.fsID = keyset.VerifyKey().Hash()
	ls.getPayInfo()

	// load lfs info first
	ls.load()

	return ls, nil
}

func (l *LfsService) Start() error {
	// start order manager
	l.OrderMgr.Start()

	go l.runPush()
	go l.persistMeta()

	has := false

	_, err := l.OrderMgr.StateGetPDPPublicKey(l.ctx, l.userID)
	if err != nil {
		time.Sleep(15 * time.Second)
		logger.Info("create fs message for: ", l.userID)

		msg := &tx.Message{
			Version: 0,
			From:    l.userID,
			To:      l.userID,
			Method:  tx.CreateFs,
			Params:  l.keyset.PublicKey().Serialize(),
		}

		l.msgChan <- msg
	} else {
		has = true
	}

	bun, err := l.OrderMgr.StateGetBucketAt(l.ctx, l.userID)
	if err != nil {
		return err
	}

	// load bucket
	l.sb.bucketVerify = bun

	for bid := uint64(0); bid < l.sb.bucketVerify; bid++ {
		if uint64(len(l.sb.buckets)) > bid {
			bu := l.sb.buckets[bid]
			bu.Confirmed = true
		} else {
			// if missing
			bu := &bucket{
				BucketInfo: types.BucketInfo{
					BucketInfo: pb.BucketInfo{
						Deletion: true,
					},
					Confirmed: true,
				},
				dirty: false,
			}
			l.sb.buckets = append(l.sb.buckets, bu)
			l.sb.NextBucketID++
		}
	}

	if l.sb.bucketVerify < l.sb.NextBucketID {
		logger.Debug("need send tx message again")
		for bid := l.sb.bucketVerify; bid < l.sb.NextBucketID; bid++ {
			logger.Debug("push create bucket message: ", bid)
			bu := l.sb.buckets[bid]

			// send bucket option
			tbp := tx.BucketParams{
				BucketOption: bu.BucketOption,
				BucketID:     bid,
			}

			data, err := tbp.Serialize()
			if err != nil {
				return err
			}

			msg := &tx.Message{
				Version: 0,
				From:    l.userID,
				To:      l.userID,
				Method:  tx.CreateBucket,
				Params:  data,
			}

			l.msgChan <- msg

			// send buc meta
			if os.Getenv("MEFS_META_UPLOAD") != "" {
				bmp := tx.BucMetaParas{
					BucketID: bid,
					Name:     bu.GetName(),
				}

				data, err = bmp.Serialize()
				if err != nil {
					return err
				}
				msg = &tx.Message{
					Version: 0,
					From:    l.userID,
					To:      l.userID,
					Method:  tx.AddBucMeta,
					Params:  data,
				}

				l.msgChan <- msg
			}
		}
	}

	for i := 0; i < int(l.sb.NextBucketID); i++ {
		bu := l.sb.buckets[i]
		if !bu.Deletion {
			go l.registerBucket(bu.BucketID, bu.NextOpID, &bu.BucketOption)
		}
	}

	logger.Debug("start lfs for: ", l.userID, l.sb.write)
	if has {
		l.sb.write = true
	}

	if l.sb.write {
		logger.Debug("lfs is ready for write")
	}

	return nil
}

func (l *LfsService) Stop() error {
	logger.Info("stop lfs service...")
	l.sb.write = false
	l.OrderMgr.Stop()
	return nil
}

func (l *LfsService) Writeable() bool {
	if l.sb == nil || l.sb.bucketNameToID == nil {
		return false
	}
	return l.sb.write
}

func (l *LfsService) LfsGetInfo(ctx context.Context, update bool) (types.LfsInfo, error) {
	if update {
		l.getPayInfo()
	}
	l.sb.RLock()
	defer l.sb.RUnlock()

	li := types.LfsInfo{
		Status: l.Writeable(),
		Bucket: uint64(len(l.sb.buckets)),
		Used:   0,
	}

	for _, bu := range l.sb.buckets {
		li.Used += bu.UsedBytes
	}

	return li, nil
}

// ShowStorage show lfs used space without appointed bucket
func (l *LfsService) ShowStorage(ctx context.Context) (uint64, error) {
	ok := l.sw.TryAcquire(1)
	if !ok {
		return 0, ErrResourceUnavailable
	}
	defer l.sw.Release(1)

	var storageSpace uint64
	for _, bucket := range l.sb.buckets {
		storageSpace += uint64(bucket.UsedBytes)
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
	if bucket.objectTree.Empty() {
		return storageSpace, nil
	}
	objectIter := bucket.objectTree.Iterator()
	for objectIter != nil {
		object, ok := objectIter.Value.(*object)
		if ok && !object.deletion {
			storageSpace += uint64(object.Size)
		}
		objectIter = objectIter.Next()

	}
	return storageSpace, nil
}
