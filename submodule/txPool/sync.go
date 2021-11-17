package txPool

import (
	"context"
	"errors"
	"sync"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/pb"
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
	tx.TxStore

	ctx context.Context
	ds  store.KVStore

	syncedHeight uint64
	remoteHeight uint64

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

		syncedHeight: 0,
		remoteHeight: 0,

		nonce: make(map[uint64]uint64),
		blks:  make(map[types.MsgID]*tx.Block),
	}
	return sp
}

func (sp *SyncPool) AddTxBlock(tb *tx.Block) error {
	if tb.Height < sp.syncedHeight {
		return ErrLowHeight
	}
	bid, err := tb.Hash()
	if err != nil {
		return err
	}

	// verify
	ok, err := sp.RoleMgr.RoleVerifyMulti(sp.ctx, bid.Bytes(), tb.MultiSignature)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidSign
	}

	// store local
	err = sp.PutTxBlock(tb)
	if err != nil {
		return err
	}

	sp.Lock()
	defer sp.Unlock()

	sp.syncedHeight = tb.Height
	sp.blks[bid] = tb

	return nil
}

func (sp *SyncPool) process() {
}

// over network
func (sp *SyncPool) GetTxBlockRemote(bid types.MsgID) (*tx.Block, error) {
	// fetch it over network
	key := store.NewKey(pb.MetaType_TX_BlockKey, bid.String())
	res, err := sp.INetService.Fetch(sp.ctx, key)
	if err != nil {
		return nil, err
	}
	tb := new(tx.Block)
	err = tb.Deserialize(res)
	return tb, err
}

func (sp *SyncPool) AddTxMsg(tb *tx.SignedMessage) error {
	return nil
}

// fetch msg over network
func (sp *SyncPool) GetTxMsgRemote(mid types.MsgID) (*tx.SignedMessage, error) {
	key := store.NewKey(pb.MetaType_TX_MessageKey, mid.String())
	res, err := sp.INetService.Fetch(sp.ctx, key)
	if err != nil {
		return nil, err
	}
	sm := new(tx.SignedMessage)
	err = sm.Deserialize(res)
	return sm, err
}
