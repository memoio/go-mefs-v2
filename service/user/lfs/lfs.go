package lfs

import (
	"context"
	"math/big"
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

	bucketChan chan uint64
	readyChan  chan struct{}
	ready      bool
}

func New(ctx context.Context, userID uint64, keyset pdpcommon.KeySet, ds store.KVStore, ss segment.SegmentStore, OrderMgr *uorder.OrderMgr) (*LfsService, error) {
	ls := &LfsService{
		ctx: ctx,

		userID: userID,
		fsID:   make([]byte, 20),

		OrderMgr: OrderMgr,
		ds:       ds,
		keyset:   keyset,

		needPay: new(big.Int),
		bal:     new(big.Int),

		sb:  newSuperBlock(),
		dps: make(map[uint64]*dataProcess),
		sw:  semaphore.NewWeighted(defaultWeighted),

		readyChan:  make(chan struct{}, 1),
		bucketChan: make(chan uint64),
	}

	ls.fsID = keyset.VerifyKey().Hash()
	ls.getPayInfo()

	return ls, nil
}

func (l *LfsService) Start() error {
	// start order manager
	l.OrderMgr.Start()

	// load lfs info first
	err := l.load()
	if err != nil {
		return err
	}

	go l.persistMeta()

	has := false

	_, err = l.OrderMgr.StateGetPDPPublicKey(l.ctx, l.userID)
	if err != nil {
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
			id, err := l.OrderMgr.PushMessage(l.ctx, msg)
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
				st, err := l.OrderMgr.SyncGetTxMsgStatus(ctx, mid)
				if err != nil {
					time.Sleep(5 * time.Second)
					continue
				}

				if st.Status.Err == 0 {
					logger.Debug("tx message done success: ", mid, msg.From, msg.To, msg.Method, st.BlockID, st.Height)
				} else {
					logger.Warn("tx message done fail: ", mid, msg.From, msg.To, msg.Method, st.BlockID, st.Height, st.Status)
				}

				break
			}
			rc <- struct{}{}
		}(mid, l.readyChan)
	} else {
		has = true
	}

	// load bucket
	l.sb.bucketVerify = l.OrderMgr.GetBucket(l.ctx, l.userID)

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

			var mid types.MsgID
			retry := 0
			for retry < 60 {
				retry++
				id, err := l.OrderMgr.PushMessage(l.ctx, msg)
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
					st, err := l.OrderMgr.SyncGetTxMsgStatus(ctx, mid)
					if err != nil {
						time.Sleep(5 * time.Second)
						continue
					}

					if st.Status.Err == 0 {
						logger.Debug("tx message done success: ", mid, msg.From, msg.To, msg.Method, st.BlockID, st.Height)
						l.bucketChan <- bucketID
					} else {
						logger.Warn("tx message done fail: ", mid, msg.From, msg.To, msg.Method, st.BlockID, st.Height, st.Status)
					}

					break
				}
			}(bid, mid)
		}
	}

	for i := 0; i < int(l.sb.NextBucketID); i++ {
		bu := l.sb.buckets[i]
		if !bu.Deletion {
			go l.OrderMgr.RegisterBucket(bu.BucketID, bu.NextOpID, &bu.BucketOption)
		}
	}

	logger.Debug("start lfs for: ", l.userID, l.ready)
	if has {
		l.ready = true
	}

	if l.ready {
		logger.Debug("lfs is ready")
	}

	return nil
}

func (l *LfsService) Stop() error {
	l.ready = false
	l.OrderMgr.Stop()
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

func (l *LfsService) LfsGetInfo(ctx context.Context, update bool) (*types.LfsInfo, error) {
	if update {
		l.getPayInfo()
	}
	l.sb.RLock()
	defer l.sb.RUnlock()

	li := &types.LfsInfo{
		Status: l.Ready(),
		Bucket: l.sb.bucketVerify,
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
