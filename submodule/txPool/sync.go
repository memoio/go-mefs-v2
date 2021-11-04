package txPool

import (
	"context"
	"errors"
	"sync"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/submodule/role"
)

var (
	ErrInvalidSign = errors.New("invalid sign")
	ErrLowHeight   = errors.New("height is low")
)

type SyncPool struct {
	sync.Mutex
	api.INetService

	*role.RoleMgr

	ctx context.Context
	ds  store.KVStore

	height uint64

	blks  map[types.MsgID]*tx.Block // need?
	nonce map[uint64]uint64
}

// sync
func NewSyncPool(ctx context.Context, ds store.KVStore, rm *role.RoleMgr, ins api.INetService) *SyncPool {
	sp := &SyncPool{
		INetService: ins,
		RoleMgr:     rm,
		ds:          ds,
		ctx:         ctx,

		nonce: make(map[uint64]uint64),
		blks:  make(map[types.MsgID]*tx.Block),
	}
	return sp
}

func (sp *SyncPool) AddBlock(tb *tx.Block) error {
	if tb.Height < sp.height {
		return ErrLowHeight
	}
	bid, err := tb.Hash()
	if err != nil {
		return err
	}

	// verify
	if !sp.RoleMgr.VerifyMulti(bid.Bytes(), tb.MultiSignature) {
		return ErrInvalidSign
	}
	// store local

	sp.Lock()
	defer sp.Unlock()

	sp.height = tb.Height
	sp.blks[bid] = tb

	return nil
}
