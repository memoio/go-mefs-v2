package txPool

import (
	"bytes"
	"context"
	"math"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/build"
	hs "github.com/memoio/go-mefs-v2/lib/hotstuff"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/submodule/state"
)

type blkDigest struct {
	height uint64
	msgs   []tx.MessageDigest
}

type SyncPool struct {
	sync.RWMutex

	api.INetService
	api.IRole
	tx.Store

	*state.StateMgr

	ctx context.Context
	ds  store.KVStore

	localID      uint64
	nextHeight   uint64 // next synced
	remoteHeight uint64 // next remote

	blks  map[uint64]*tx.SignedBlock // key: height
	nonce map[uint64]uint64          // key: roleID

	ready bool

	msgDone chan *tx.MessageDigest
	inPush  bool

	msgChan   chan *tx.SignedMessage
	blkDone   chan *blkDigest
	inProcess bool
}

// sync
func NewSyncPool(ctx context.Context, roleID uint64, st *state.StateMgr, ds store.KVStore, ts tx.Store, ir api.IRole, ins api.INetService) *SyncPool {
	sp := &SyncPool{
		INetService: ins,
		IRole:       ir,
		Store:       ts,

		StateMgr: st,

		ds:  ds,
		ctx: ctx,

		localID:      roleID,
		nextHeight:   0,
		remoteHeight: 0,

		nonce: make(map[uint64]uint64),
		blks:  make(map[uint64]*tx.SignedBlock),

		msgChan: make(chan *tx.SignedMessage, 128),
		msgDone: make(chan *tx.MessageDigest, 16),
		blkDone: make(chan *blkDigest, 8),
	}

	sp.load()

	return sp
}

func (sp *SyncPool) Start() {
	logger.Debug("start sync pool")
	go sp.syncBlock()
}

func (sp *SyncPool) SetReady() {
	sp.Lock()
	sp.ready = true
	sp.Unlock()
}

func (sp *SyncPool) load() {
	// todo: handle case if msglen > 0
	ht := sp.GetHeight(sp.ctx)
	msglen := sp.GetMsgNum()
	if msglen != 0 {
		logger.Warn("state is incomplete at: ", ht, msglen)
	}
	sp.nextHeight = ht
	sp.remoteHeight = ht
	if sp.nextHeight == 0 {
		sp.PutTxBlockHeight(math.MaxUint64, build.GenesisBlockID("test"))
	}

	logger.Debug("block synced to: ", sp.nextHeight)
}

func (sp *SyncPool) syncBlock() {
	tc := time.NewTicker(1 * time.Second)
	defer tc.Stop()

	for {
		select {
		case <-sp.ctx.Done():
			return
		case <-tc.C:
		}

		if sp.remoteHeight == sp.nextHeight {
			logger.Debug("regular handle block synced at:", sp.nextHeight)
			continue
		}

		logger.Debug("regular get block:", sp.nextHeight, sp.remoteHeight)

		// from end -> begin
		// sync all block head
		for i := sp.remoteHeight - 1; i >= sp.nextHeight && i < math.MaxUint64; i-- {
			sp.RLock()
			_, ok := sp.blks[i]
			sp.RUnlock()
			if !ok {
				// sync block from remote
				bid, err := sp.GetTxBlockByHeight(i)
				if err != nil {
					logger.Debug("get block height fail: ", i, err)
					continue
				}

				blk, err := sp.GetTxBlock(bid)
				if err != nil {
					blk, err = sp.GetTxBlockRemote(bid)
					if err != nil {
						logger.Debug("get block remote fail: ", i, bid, err)
						continue
					}
				}

				sp.Lock()
				sp.blks[i] = blk
				sp.Unlock()
			}
		}

		rh := sp.remoteHeight
		if rh > sp.nextHeight+128 {
			rh = sp.nextHeight + 128
		}

		logger.Debug("regular process block:", sp.nextHeight, rh, sp.remoteHeight)

		// process syncd blk
		for i := sp.nextHeight; i < rh; i++ {
			sp.Lock()
			sb, ok := sp.blks[i]
			if ok {
				err := sp.processTxBlock(sb)
				if err != nil {
					// clear all block above sp.nextHeight
					for j := i; j < sp.remoteHeight; j++ {
						delete(sp.blks, j)
					}
					sp.remoteHeight = i
					sp.Unlock()
					logger.Debug(err)
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
		}

		sp.Lock()
		if sp.nextHeight > sp.remoteHeight {
			sp.remoteHeight = sp.nextHeight
		}
		sp.Unlock()
	}
}

func (sp *SyncPool) processTxBlock(sb *tx.SignedBlock) error {
	bid := sb.Hash()
	logger.Debug("process block: ", sb.Height, bid)
	oRoot, err := sp.AppleyMsg(nil, nil)
	if err != nil {
		return err
	}

	if !bytes.Equal(oRoot.Bytes(), sb.ParentRoot.Bytes()) {
		logger.Warnf("local has wrong state, got: %s, expected: %s", oRoot, sb.ParentRoot)
	}

	newRoot, err := sp.ApplyBlock(sb)
	if err != nil {
		logger.Debug("apply block fail: ", err)
		return err
	}

	mds := &blkDigest{
		height: sb.Height,
		msgs:   make([]tx.MessageDigest, 0, len(sb.Msgs)),
	}

	for i, msg := range sb.Msgs {
		nextNonce, ok := sp.nonce[msg.From]
		if !ok {
			nextNonce = sp.GetNonce(sp.ctx, msg.From)
		}

		if nextNonce != msg.Nonce {
			logger.Debug("has wrong nonce: ", msg.From, msg.Nonce, nextNonce)
		}

		sp.nonce[msg.From] = msg.Nonce + 1

		// apply message
		newRoot, err = sp.AppleyMsg(&msg.Message, &sb.Receipts[i])
		if err != nil {
			// should not; todo
			logger.Error("apply message fail: ", msg.From, msg.Nonce, msg.Method, err)
		}

		ms := &tx.MsgState{
			BlockID: bid,
			Height:  sb.Height,
			Status:  sb.Receipts[i],
		}

		msb, err := ms.Serialize()
		if err != nil {
			return err
		}

		key := store.NewKey(pb.MetaType_Tx_MessageStateKey, msg.Hash().String())
		sp.ds.Put(key, msb)

		md := tx.MessageDigest{
			ID:    msg.Hash(),
			From:  msg.From,
			Nonce: msg.Nonce,
		}

		mds.msgs = append(mds.msgs, md)

		if msg.From == sp.localID {
			if sp.inPush {
				sp.msgDone <- &md
			}
		}
	}

	if !bytes.Equal(newRoot.Bytes(), sb.Root.Bytes()) {
		logger.Warnf("local has wrong state, got: %s, expected: %s", newRoot, sb.Root)
	}

	if sp.inProcess {
		sp.blkDone <- mds
	}

	logger.Debug("process block done: ", sb.Height, bid)
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

func (sp *SyncPool) GetTxMsgStatus(ctx context.Context, mid types.MsgID) (*tx.MsgState, error) {
	return sp.Store.GetTxMsgState(mid)
}

func (sp *SyncPool) AddTxBlock(tb *tx.SignedBlock) error {
	logger.Debug("add block: ", tb.Height, sp.nextHeight, sp.remoteHeight)
	if tb.Height < sp.nextHeight {
		return xerrors.Errorf("height expected %d, got %d", sp.nextHeight, tb.Height)
	}

	sp.SetReady()

	sp.RLock()
	_, ok := sp.blks[tb.Height]
	if ok {
		sp.RUnlock()
		return nil
	}
	sp.RUnlock()

	bid := tb.Hash()

	has, _ := sp.HasTxBlock(bid)
	if has {
		logger.Debug("add tx block, already have")
		return nil
	}

	// verify
	ok, err := sp.RoleVerifyMulti(sp.ctx, hs.CalcHash(bid.Bytes(), hs.PhaseCommit), tb.MultiSignature)
	if err != nil {
		for _, signer := range tb.Signer {
			go sp.getRoleInfoRemote(signer)
		}
		return err
	}
	if !ok {
		return xerrors.Errorf("%s block at height %d sign is invalid", bid, tb.Height)
	}

	// verify all msg
	for _, msg := range tb.Msgs {
		ok, err := sp.RoleVerify(sp.ctx, msg.From, msg.Hash().Bytes(), msg.Signature)
		if err != nil {
			go sp.getRoleInfoRemote(msg.From)
			return err
		}

		if !ok {
			return xerrors.Errorf("%s block at height %d msg %d sign is invalid", bid, tb.Height, msg.From)
		}
	}

	// store local
	err = sp.PutTxBlock(tb)
	if err != nil {
		logger.Debug("add block: ", err)
		return err
	}

	sp.Lock()
	if tb.Height >= sp.nextHeight {
		sp.blks[tb.Height] = tb
	}

	if tb.Height >= sp.remoteHeight {
		sp.remoteHeight = tb.Height + 1
	}
	sp.Unlock()

	return nil
}

// over network
func (sp *SyncPool) GetTxBlockRemote(bid types.MsgID) (*tx.SignedBlock, error) {
	// fetch it over network
	key := store.NewKey(pb.MetaType_TX_BlockKey, bid.String())
	res, err := sp.INetService.Fetch(sp.ctx, key)
	if err != nil {
		return nil, err
	}
	tb := new(tx.SignedBlock)
	err = tb.Deserialize(res)
	if err != nil {
		return nil, err
	}

	return tb, sp.AddTxBlock(tb)
}

func (sp *SyncPool) AddTxMsg(ctx context.Context, msg *tx.SignedMessage) error {
	mid := msg.Hash()
	ok, err := sp.HasTxMsg(mid)
	if err != nil || !ok {
		valid, err := sp.RoleSanityCheck(ctx, msg)
		if err != nil {
			return err
		}

		if !valid {
			return xerrors.Errorf("msg is invalid")
		}

		ok, err := sp.RoleVerify(ctx, msg.From, mid.Bytes(), msg.Signature)
		if err != nil {
			logger.Debug("add tx msg:", msg.From, mid, err)
			return err
		}

		if !ok {
			return xerrors.Errorf("%d %d tx msg %s sign invalid", msg.From, msg.Nonce, mid)
		}

		return sp.PutTxMsg(msg)
	}
	return nil
}

// fetch msg over network
func (sp *SyncPool) getTxMsgRemote(mid types.MsgID) (*tx.SignedMessage, error) {
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
