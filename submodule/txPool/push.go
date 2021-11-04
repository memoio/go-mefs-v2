package txPool

import (
	"context"
	"sync"
	"time"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

type PushPool struct {
	sync.Mutex
	api.IWallet // sign message
	api.INetService

	ctx context.Context
	ds  store.KVStore

	roleID uint64
	wallet address.Address // for sign
	bls    address.Address

	min     uint64                    // pending
	max     uint64                    // pending
	pending map[types.MsgID]time.Time // topush
}

func NewPushPool(ctx context.Context, ds store.KVStore) *PushPool {
	pp := &PushPool{}
	return pp
}

func (pp *PushPool) AddMessage(mes *tx.Message) error {
	// get nonce
	// sign
	// store
	// push out immediately or regular

	sm := &tx.SignedMessage{
		Message: *mes,
	}
	return pp.INetService.PublishTxMsg(pp.ctx, sm)
}

func (pp *PushPool) Push(force bool) {

}

func (pp *PushPool) Sync() {
	// sync block and update nonce
}
