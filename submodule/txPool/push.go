package txPool

import (
	"sync"

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

	ds store.KVStore

	roleID uint64
	wallet address.Address // for sign
	bls    address.Address

	nonce   uint64                            // confirmed
	min     uint64                            // pending
	max     uint64                            // pending
	pending map[types.MsgID]*tx.SignedMessage // topush
}

func NewPushPool() *PushPool {
	pp := &PushPool{}
	return pp
}

func (pp *PushPool) AddMessage() {
	// add nonce
	// sign
	// store
	// push out immediately or regular
}

func (pp *PushPool) Push() {

}

func (pp *PushPool) Sync() {
	// sync block and update nonce
}
