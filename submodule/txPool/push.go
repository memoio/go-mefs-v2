package txPool

import (
	"context"
	"sync"
	"time"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/submodule/role"
)

type PushPool struct {
	sync.Mutex
	api.IWallet // sign message
	api.INetService

	*role.RoleMgr

	ctx context.Context
	ds  store.KVStore

	min     uint64                    // pending
	max     uint64                    // pending
	pending map[types.MsgID]time.Time // topush
}

func NewPushPool(ctx context.Context, ds store.KVStore, rm *role.RoleMgr, iw api.IWallet, ins api.INetService) *PushPool {
	pp := &PushPool{
		IWallet:     iw,
		INetService: ins,
		RoleMgr:     rm,
		ds:          ds,
		ctx:         ctx,
	}
	return pp
}

func (pp *PushPool) AddMessage(mes *tx.Message) error {
	// get nonce
	// sign
	// store
	// push out immediately or regular

	pp.Lock()
	defer pp.Unlock()

	mes.Nonce = pp.max
	mid, err := mes.Hash()
	if err != nil {
		return err
	}

	sig, err := pp.RoleSign(pp.ctx, mid.Bytes(), types.SigSecp256k1)
	if err != nil {
		return err
	}

	sm := &tx.SignedMessage{
		Message:   *mes,
		Signature: sig,
	}

	pp.max++
	pp.pending[mid] = time.Now()

	// store local

	return pp.INetService.PublishTxMsg(pp.ctx, sm)
}

func (pp *PushPool) Push(force bool) {
	// push pending again
}

func (pp *PushPool) UpdateNonce(nonce uint64) {
	pp.min = nonce
}

func (pp *PushPool) GetPendingMsg() []types.MsgID {
	return nil
}

func (pp *PushPool) Sync() {
	// sync block and update nonce
}
