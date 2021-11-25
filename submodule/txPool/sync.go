package txPool

import (
	"context"
	"encoding/binary"
	"math"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

type SyncedBlock struct {
	tx.BlockHeader
	msg      []*tx.Message
	msgCount int
}

type SyncPool struct {
	sync.RWMutex

	api.INetService
	api.IRole
	tx.Store

	ctx context.Context
	ds  store.KVStore

	localID      uint64
	nextHeight   uint64 // next synced
	remoteHeight uint64 // next remote

	blks  map[uint64]*SyncedBlock // key: height
	nonce map[uint64]uint64       // key: roleID

	syncChan chan struct{}

	mf HandlerMessageFunc

	ready bool

	msgDone chan *tx.MessageDigest
	inPush  bool

	blkDone   chan *tx.BlockHeader
	inProcess bool
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
		blks:  make(map[uint64]*SyncedBlock),

		syncChan: make(chan struct{}),
		msgDone:  make(chan *tx.MessageDigest, 16),
		blkDone:  make(chan *tx.BlockHeader, 8),
	}

	sp.load()

	go sp.sync()

	return sp
}

func (sp *SyncPool) load() {
	key := store.NewKey(pb.MetaType_Tx_BlockSyncedKey)
	val, err := sp.ds.Get(key)
	if err == nil && len(val) >= 8 {
		sp.nextHeight = binary.BigEndian.Uint64(val)
		sp.remoteHeight = sp.nextHeight
	}

	if sp.nextHeight == 0 {
		sp.PutTxBlockHeight(math.MaxUint64, build.GenesisBlockID("test"))
	}

	logger.Debug("block sync to: ", sp.nextHeight)
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

		if sp.remoteHeight == sp.nextHeight {
			logger.Debug("regular handle block synced at:", sp.nextHeight)
			continue
		}

		logger.Debug("regular handle block:", sp.nextHeight, sp.remoteHeight)

		// from end -> begin
		for i := sp.remoteHeight - 1; i >= sp.nextHeight && i < math.MaxUint64; i-- {
			sb, ok := sp.blks[i]
			if !ok {
				// sync block from remote

				bid, err := sp.GetTxBlockByHeight(i)
				if err != nil {
					logger.Debug("get block height fail: ", i, err)
					continue
				}

				blk, err := sp.GetTxBlock(bid)
				if err != nil {
					logger.Debug("get block fail: ", i, bid, err)
					go sp.GetTxBlockRemote(bid)
					continue
				}

				sb = &SyncedBlock{
					BlockHeader: blk.BlockHeader,
					msg:         make([]*tx.Message, len(blk.Txs)),
					msgCount:    0,
				}

				sp.Lock()
				sp.blks[i] = sb
				sp.Unlock()

				sp.PutTxBlockHeight(i-1, blk.PrevID)
			}

			// sync all msg of one block
			for j, tx := range sb.Txs {
				if sb.msg[j] != nil {
					continue
				}
				sm, err := sp.GetTxMsg(tx.ID)
				if err != nil {
					go sp.getRoleInfoRemote(tx.From)
					go sp.GetTxMsgRemote(tx.ID)
				} else {
					sb.msg[j] = &sm.Message
					sb.msgCount++
				}
			}
		}

		if sp.mf == nil {
			logger.Debug("regular process block need msg handle")
			continue
		}

		logger.Debug("regular process block:", sp.nextHeight, sp.remoteHeight)

		// process syncd blk
		for i := sp.nextHeight; i < sp.remoteHeight; i++ {
			sp.Lock()
			blk, ok := sp.blks[i]
			if ok {
				if len(blk.Txs) > blk.msgCount {
					logger.Debug("before process block: ", len(blk.Txs), blk.msgCount)
					sp.Unlock()
					break
				}
				err := sp.processTxBlock(blk)
				if err != nil {
					sp.Unlock()
					logger.Debug("block is wrong, should not", err)
					break
				}
				delete(sp.blks, i)
			} else {
				logger.Debug("before process block, not have")
				sp.Unlock()
				break
			}
			sp.nextHeight++
			sp.Unlock()
			binary.BigEndian.PutUint64(buf, i+1)
			sp.ds.Put(key, buf)
		}

		sp.Lock()
		if sp.nextHeight > sp.remoteHeight {
			sp.remoteHeight = sp.nextHeight
		}
		sp.Unlock()
	}
}

func (sp *SyncPool) processTxBlock(sb *SyncedBlock) error {
	logger.Debug("process block:", sb.Height)
	buf := make([]byte, 8)
	for i := 0; i < sb.msgCount; i++ {
		tx := sb.Txs[i]

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
		if sb.Receipts[i].Err == 0 {
			err := sp.mf(sb.msg[i])
			if err != nil {
				// should not; todo
				logger.Error("fail to apply: ", err)
			}
		}

		binary.BigEndian.PutUint64(buf, tx.Nonce+1)
		sp.ds.Put(key, buf)

		if tx.From == sp.localID {
			if sp.inPush {
				sp.msgDone <- &tx
			}
		}
	}

	if sp.inProcess {
		sp.blkDone <- &sb.BlockHeader
	}

	return nil
}

func (sp *SyncPool) GetSyncStatus(ctx context.Context) bool {
	if sp.nextHeight == sp.remoteHeight && sp.ready {
		return true
	}
	return false
}

func (sp *SyncPool) GetSyncHeight(ctx context.Context) (uint64, uint64) {
	return sp.nextHeight, sp.remoteHeight
}

func (sp *SyncPool) GetNonce(ctx context.Context, from uint64) uint64 {
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

func (sp *SyncPool) GetTxMsgStatus(ctx context.Context, mid types.MsgID) (*tx.MsgState, error) {
	return sp.Store.GetTxMsgState(mid)
}

func (sp *SyncPool) AddTxBlock(tb *tx.Block) error {
	logger.Debug("add block: ", tb.Height, sp.nextHeight, sp.remoteHeight)
	if tb.Height < sp.nextHeight {
		return ErrLowHeight
	}

	sp.Lock()
	sp.ready = true // got new block
	sp.Unlock()

	sp.RLock()
	_, ok := sp.blks[tb.Height]
	if ok {
		sp.RUnlock()
		return nil
	}
	sp.RUnlock()

	bid, err := tb.Hash()
	if err != nil {
		return err
	}

	has, _ := sp.HasTxBlock(bid)
	if has {
		logger.Debug("add block has: ")
		return nil
	}

	// verify
	ok, err = sp.RoleVerifyMulti(sp.ctx, bid.Bytes(), tb.MultiSignature)
	if err != nil {
		logger.Debug("add block: ", err)
		return err
	}
	if !ok {
		logger.Debug("add block invalid sign")
		return ErrInvalidSign
	}

	// store local
	err = sp.PutTxBlock(tb)
	if err != nil {
		logger.Debug("add block: ", err)
		return err
	}

	sp.Lock()
	if tb.Height >= sp.nextHeight {
		sb := &SyncedBlock{
			BlockHeader: tb.BlockHeader,
			msg:         make([]*tx.Message, len(tb.Txs)),
			msgCount:    0,
		}

		sp.blks[tb.Height] = sb
	}

	if tb.Height >= sp.remoteHeight {
		sp.remoteHeight = tb.Height + 1
	}
	sp.Unlock()

	sp.syncChan <- struct{}{}

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

func (sp *SyncPool) AddTxMsg(ctx context.Context, tb *tx.SignedMessage) error {
	mid, err := tb.Hash()
	if err != nil {
		logger.Debug("add tx msg:", tb.From, err)
		return err
	}

	ok, err := sp.HasTxMsg(mid)
	if err == nil && ok {
		return nil
	}

	ok, err = sp.RoleVerify(ctx, tb.From, mid.Bytes(), tb.Signature)
	if err != nil {
		logger.Debug("add tx msg:", tb.From, mid, err)
		return err
	}

	if !ok {
		logger.Debug("add tx msg:", tb.From, mid, ErrInvalidSign)
		return ErrInvalidSign
	}

	return sp.PutTxMsg(tb)
}

// fetch msg over network
func (sp *SyncPool) GetTxMsgRemote(mid types.MsgID) (*tx.SignedMessage, error) {
	key := store.NewKey(pb.MetaType_TX_MessageKey, mid.String())
	res, err := sp.INetService.Fetch(sp.ctx, key)
	if err != nil {
		logger.Debug("get tx msg from remote: ", mid, err)
		return nil, err
	}
	sm := new(tx.SignedMessage)
	err = sm.Deserialize(res)
	if err != nil {
		logger.Debug("get tx msg from remote: ", mid, err)
		return nil, err
	}

	logger.Debug("get tx msg from remote: ", mid)
	return sm, sp.AddTxMsg(sp.ctx, sm)
}

func (sp *SyncPool) RegisterMsgFunc(h HandlerMessageFunc) {
	sp.Lock()
	sp.mf = h
	sp.Unlock()
}

func (sp *SyncPool) getRoleInfoRemote(roleID uint64) {
	key := store.NewKey(pb.MetaType_RoleInfoKey, roleID)
	ok, err := sp.ds.Has(key)
	if err == nil && ok {
		return
	}

	mes, err := sp.INetService.Fetch(sp.ctx, key)
	if err == nil && len(mes) > 0 {
		ri := new(pb.RoleInfo)
		err := proto.Unmarshal(mes, ri)
		if err == nil {
			sp.ds.Put(key, mes)
		}
	}
}
