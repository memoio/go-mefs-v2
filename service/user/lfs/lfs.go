package lfs

import (
	"context"
	"encoding/binary"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"

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

	om *uorder.OrderMgr

	ctx    context.Context
	keyset pdpcommon.KeySet
	ds     store.KVStore

	userID     uint64
	fsID       []byte // keyset的verifyKey的hash
	encryptKey []byte

	sb *superBlock

	dps map[uint64]*dataProcess

	sw *semaphore.Weighted // manage resource

	bucketChan chan uint64
	readyChan  chan struct{}
	ready      bool
}

func New(ctx context.Context, userID uint64, keyset pdpcommon.KeySet, ds store.KVStore, ss segment.SegmentStore, om *uorder.OrderMgr) (*LfsService, error) {
	ls := &LfsService{
		ctx: ctx,

		userID: userID,
		fsID:   make([]byte, 20),

		om:     om,
		ds:     ds,
		keyset: keyset,

		sb:  newSuperBlock(),
		dps: make(map[uint64]*dataProcess),
		sw:  semaphore.NewWeighted(defaultWeighted),

		readyChan: make(chan struct{}, 1),
	}

	ls.fsID = keyset.VerifyKey().Hash()

	// load lfs info first
	err := ls.Load()
	if err != nil {
		return nil, err
	}

	go ls.persistMeta()

	return ls, nil
}

func (l *LfsService) Start() error {
	// start order manager
	l.om.Start()

	has := false
	ok, err := l.ds.Has(store.NewKey(pb.MetaType_ST_PDPPublicKey, l.userID))
	if err != nil || !ok {
		time.Sleep(15 * time.Second)
		logger.Debug("push create fs message for: ", l.userID)

		msg := &tx.Message{
			Version: 0,
			From:    l.userID,
			To:      l.userID,
			Method:  tx.CreateFs,
			Params:  l.keyset.PublicKey().Serialize(),
		}

		var mid types.MsgID
		for {
			id, err := l.om.PushMessage(l.ctx, msg)
			if err != nil {
				time.Sleep(5 * time.Second)
				continue
			}
			mid = id
			break
		}

		go func(mid types.MsgID, rc chan struct{}) {
			ctx, cancle := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancle()
			for {
				st, err := l.om.GetTxMsgStatus(ctx, mid)
				if err != nil {
					time.Sleep(5 * time.Second)
					continue
				}

				logger.Debug("tx message done: ", mid, st.BlockID, st.Height, st.Status.Err, string(st.Status.Extra))
				break
			}
			rc <- struct{}{}
		}(mid, l.readyChan)
	} else {
		has = true
	}

	// load bucket
	data, err := l.ds.Get(store.NewKey(pb.MetaType_ST_BucketOptKey, l.userID))
	if err == nil && len(data) >= 8 {
		l.sb.bucketVerify = binary.BigEndian.Uint64(data)
	}
	if l.sb.bucketVerify < l.sb.NextBucketID {
		logger.Debug("need send tx message again")
		for bid := l.sb.bucketVerify; bid < l.sb.NextBucketID; bid++ {
			logger.Debug("push create bucket message: ", bid)
			bu := l.sb.buckets[bid]
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

			// handle result and retry?

			var mid types.MsgID
			retry := 0
			for retry < 60 {
				retry++
				id, err := l.om.PushMessage(l.ctx, msg)
				if err != nil {
					time.Sleep(10 * time.Second)
					continue
				}
				mid = id
				break
			}

			go func(bucketID uint64, mid types.MsgID) {
				ctx, cancle := context.WithTimeout(context.Background(), 10*time.Minute)
				defer cancle()
				logger.Debug("waiting tx message done: ", mid)

				for {
					st, err := l.om.GetTxMsgStatus(ctx, mid)
					if err != nil {
						time.Sleep(5 * time.Second)
						continue
					}

					logger.Debug("tx message done: ", mid, st.BlockID, st.Height, st.Status.Err, string(st.Status.Extra))
					break
				}

				l.bucketChan <- bucketID
			}(bid, mid)
		}
	}

	for i := 0; i < int(l.sb.NextBucketID); i++ {
		bu := l.sb.buckets[i]
		if !bu.Deletion {
			go l.om.RegisterBucket(bu.BucketID, bu.NextOpID, &bu.BucketOption)
		}
	}

	if has {
		l.ready = true
	}
	logger.Debug("start lfs for: ", l.userID)

	return nil
}

func (l *LfsService) Stop() error {
	l.ready = false
	return nil
}

func (l *LfsService) Ready() bool {
	if l.sb == nil || l.sb.bucketNameToID == nil {
		return false
	}
	return l.ready
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
