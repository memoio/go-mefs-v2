package txPool

import (
	"context"
	"sync"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

type MsgInfo struct {
	state uint8       // 1: in pool; 2: added to block; 3: confirmed in block
	id    types.MsgID // message hash
}

type msgSet struct {
	nonce   uint64              // next process or
	min     uint64              // for confirm and delete
	largest uint64              // pending nonce in pool >= next
	info    map[uint64]*MsgInfo // all currently processable tx
}

func (ms *msgSet) CanProcess() []types.MsgID {
	res := make([]types.MsgID, 0, ms.largest-ms.nonce)
	for n := ms.nonce; n <= ms.largest; n++ {
		mi, ok := ms.info[n]
		if ok {
			res = append(res, mi.id)
		} else {
			return res
		}
	}
	return res
}

type InPool struct {
	sync.Mutex
	api.IWallet
	api.IRole

	ctx context.Context
	ds  store.KVStore

	txMap   map[types.MsgID]*tx.Message // key: msgID; value: tx.Message
	pending map[uint64]*msgSet          // key: from; all currently processable tx

	wallet map[uint64]address.Address // cache: roleID<->wallet address
}

func NewPool(ctx context.Context, ds store.KVStore) *InPool {
	pl := &InPool{
		ctx:     ctx,
		ds:      ds,
		pending: make(map[uint64]*msgSet),
	}

	return pl
}

func (mp *InPool) AddMsg(m *tx.SignedMessage) error {
	// check exustence
	// validate
	// add message to map
	// try add message to pending;
	// 1. not has this nonce; add to pending and verify > nonce again
	// 2. has this nonce: replace it if has larger gas and verify > nonce again

	// add to queue
	return nil
}

func (mp *InPool) ValidateMsg(m *tx.SignedMessage) bool {
	// nonce < next;

	// add to datastore

	return false
}
