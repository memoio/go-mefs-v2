package txPool

import (
	"context"
	"encoding/binary"
	"sync"
	"time"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

type SyncdBlock struct {
	tx.BlockHeader
	msgCount int
}

type SyncPool struct {
	sync.Mutex

	api.INetService
	api.IRole
	tx.Store

	ctx context.Context
	ds  store.KVStore

	localID      uint64
	nextHeight   uint64
	remoteHeight uint64

	blks  map[uint64]*SyncdBlock // key: height
	nonce map[uint64]uint64      // key: roleID

	syncChan chan struct{}
	msgDone  chan types.MsgID
	blkDone  chan *tx.BlockHeader
}

// sync
func NewSyncPool(ctx context.Context, roleID uint64, ds store.KVStore, ts tx.Store, ir api.IRole, ins api.INetService) *SyncPool {
	sp := &SyncPool{
		INetService: ins,
		IRole:       ir,
		Store:       ts,

		ds:  ds,
		ctx: ctx,

		localID:      roleID,
		nextHeight:   0,
		remoteHeight: 0,

		nonce: make(map[uint64]uint64),
		blks:  make(map[uint64]*SyncdBlock),

		syncChan: make(chan struct{}),
		msgDone:  make(chan types.MsgID, 16),
		blkDone:  make(chan *tx.BlockHeader, 8),
	}

	sp.load()

	go sp.sync()

	return sp
}

func (sp *SyncPool) load() {
	key := store.NewKey(pb.MetaType_Tx_BlockSyncedKey)
	val, err := sp.ds.Get(key)
	if err != nil {
		return
	}
	if len(val) >= 8 {
		sp.nextHeight = binary.BigEndian.Uint64(val)
		sp.remoteHeight = sp.nextHeight
	}
}

func (sp *SyncPool) sync() {
	tc := time.NewTicker(10 * time.Second)
	defer tc.Stop()

	key := store.NewKey(pb.MetaType_Tx_BlockSyncedKey)
	buf := make([]byte, 8)
	for {
		select {
		case <-sp.ctx.Done():
			return
		case <-tc.C:
		case <-sp.syncChan:
		}

		logger.Debug("handle block in pool:", sp.nextHeight, sp.remoteHeight)

		for i := sp.nextHeight; i <= sp.remoteHeight; i++ {
			sb, ok := sp.blks[i]
			if !ok {
				// sync block from remote
				bid, err := sp.GetTxBlockByHeight(i)
				if err != nil {
					continue
				}

				blk, err := sp.GetTxBlock(bid)
				if err != nil {
					go sp.GetTxBlockRemote(bid)
					continue
				}

				sb = &SyncdBlock{
					BlockHeader: blk.BlockHeader,
					msgCount:    0,
				}

				sp.Lock()
				sp.blks[i] = sb
				sp.Unlock()
			}

			// sync all msg of one block
			for _, tx := range sb.Txs {
				has, err := sp.HasTxMsg(tx.ID)
				if err != nil || !has {
					go sp.GetTxMsgRemote(tx.ID)
				} else {
					sb.msgCount++
				}
			}
		}

		// process syncd blk
		for i := sp.nextHeight; i <= sp.remoteHeight; i++ {
			sp.Lock()
			blk, ok := sp.blks[i]
			if ok {
				if len(blk.Txs) > blk.msgCount {
					sp.Unlock()
					break
				}
				err := sp.processTxBlock(&blk.BlockHeader)
				if err != nil {
					sp.Unlock()
					logger.Debug("blk is wrong, should not")
					break
				}
				delete(sp.blks, i)
			} else {
				sp.Unlock()
				break
			}

			sp.nextHeight++
			logger.Debug("block is synced to:", sp.nextHeight)
			sp.Unlock()
			binary.BigEndian.PutUint64(buf, i+1)
			sp.ds.Put(key, buf)
		}
	}
}

func (sp *SyncPool) processTxBlock(tb *tx.BlockHeader) error {
	logger.Debug("process block:", tb.Height)
	id, err := tb.Hash()
	if err != nil {
		return err
	}
	ms := &tx.MessageState{
		BlockID: id,
		Height:  tb.Height,
	}

	msb, err := ms.Serialize()
	if err != nil {
		return err
	}

	buf := make([]byte, 8)

	for _, tx := range tb.Txs {
		key := store.NewKey(pb.MetaType_Tx_NonceKey, tx.From)

		nextNonce, ok := sp.nonce[tx.From]
		if !ok {
			val, err := sp.ds.Get(key)
			if err == nil && len(val) >= 8 {
				nextNonce = binary.BigEndian.Uint64(val)
			}
		}

		if nextNonce != tx.Nonce {
			logger.Debug("has nonce: ", tx.From, tx.Nonce, nextNonce)
		}

		sp.nonce[tx.From] = tx.Nonce + 1

		// apply message

		binary.BigEndian.PutUint64(buf, tx.Nonce+1)
		sp.ds.Put(key, buf)

		key = store.NewKey(pb.MetaType_Tx_MessageStateKey, tx.ID.Bytes())
		sp.ds.Put(key, msb)

		if tx.From == sp.localID {
			sp.msgDone <- tx.ID
		}
	}

	sp.blkDone <- tb

	return nil
}

func (sp *SyncPool) GetSyncStatus() bool {
	if sp.nextHeight <= sp.remoteHeight && sp.remoteHeight-sp.nextHeight < 3 {
		return true
	}
	return false
}

func (sp *SyncPool) GetSyncHeight() (uint64, uint64) {
	return sp.nextHeight, sp.remoteHeight
}

func (sp *SyncPool) GetNextNonce(from uint64) uint64 {
	nextNonce, ok := sp.nonce[from]
	if ok {
		return nextNonce
	} else {
		key := store.NewKey(pb.MetaType_Tx_NonceKey, from)
		val, err := sp.ds.Get(key)
		if err == nil && len(val) >= 8 {
			nextNonce = binary.BigEndian.Uint64(val)
			sp.Lock()
			sp.nonce[from] = nextNonce
			sp.Unlock()
			return nextNonce
		}
	}
	return 0
}

func (sp *SyncPool) AddTxBlock(tb *tx.Block) error {
	logger.Debug("add block: ", tb.Height, sp.nextHeight, sp.remoteHeight)
	if tb.Height < sp.nextHeight {
		return ErrLowHeight
	}

	bid, err := tb.Hash()
	if err != nil {
		return err
	}

	has, _ := sp.HasTxBlock(bid)
	if has {
		return nil
	}

	// verify
	ok, err := sp.RoleVerifyMulti(sp.ctx, bid.Bytes(), tb.MultiSignature)
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

	if tb.Height >= sp.nextHeight {
		sb := &SyncdBlock{
			tb.BlockHeader, 0,
		}

		sp.blks[tb.Height] = sb
		if tb.Height > sp.remoteHeight {
			sp.remoteHeight = tb.Height
		}
	}

	return nil
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
	if err != nil {
		return nil, err
	}

	return tb, sp.AddTxBlock(tb)
}

func (sp *SyncPool) AddTxMsg(tb *tx.SignedMessage) error {
	mid, err := tb.Hash()
	if err != nil {
		return err
	}

	ok, err := sp.HasTxMsg(mid)
	if ok && err == nil {
		return nil
	}

	ok, err = sp.RoleVerify(sp.ctx, tb.From, mid.Bytes(), tb.Signature)
	if err != nil {
		return err
	}

	if !ok {
		return ErrInvalidSign
	}

	return sp.PutTxMsg(tb)
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
	if err != nil {
		return nil, err
	}
	return sm, sp.AddTxMsg(sm)
}

func (sp *SyncPool) applyMsg(mes *tx.Message) error {
	return nil
}
