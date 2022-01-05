package txPool

import (
	"bytes"
	"context"
	"math"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
	"go.opencensus.io/stats"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/build"
	hs "github.com/memoio/go-mefs-v2/lib/hotstuff"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/submodule/metrics"
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

	thre         int
	localID      uint64
	groupID      uint64
	nextHeight   uint64 // next synced
	remoteHeight uint64 // next remote

	blks  map[uint64]types.MsgID // key: height
	nonce map[uint64]uint64      // key: roleID

	ready bool

	msgDone chan *tx.MessageDigest
	inPush  bool

	msgChan   chan *tx.SignedMessage
	blkDone   chan *blkDigest
	inProcess bool

	retry int
	bads  map[types.MsgID]struct{}
}

// sync
func NewSyncPool(ctx context.Context, roleID, groupID uint64, thre int, st *state.StateMgr, ds store.KVStore, ts tx.Store, ir api.IRole, ins api.INetService) *SyncPool {
	sp := &SyncPool{
		INetService: ins,
		IRole:       ir,
		Store:       ts,

		StateMgr: st,

		ds:  ds,
		ctx: ctx,

		thre:         thre,
		localID:      roleID,
		groupID:      groupID,
		nextHeight:   0,
		remoteHeight: 0,

		nonce: make(map[uint64]uint64),
		blks:  make(map[uint64]types.MsgID),

		msgChan: make(chan *tx.SignedMessage, 128),
		msgDone: make(chan *tx.MessageDigest, 16),
		blkDone: make(chan *blkDigest, 8),

		bads: make(map[types.MsgID]struct{}),
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
	sp.nextHeight = ht
	sp.remoteHeight = ht
	if sp.nextHeight == 0 {
		sp.PutTxBlockHeight(math.MaxUint64, build.GenesisBlockID("test"))
	}

	logger.Debug("block synced to: ", sp.nextHeight)

	msglen := sp.GetMsgNum()
	if msglen != 0 {
		logger.Error("state is incomplete at: ", ht, msglen)
	}
}

func (sp *SyncPool) syncBlock() {
	tc := time.NewTicker(3 * time.Second)
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

		// sync all block from end -> begin
		// todo: use prevID to find
		for i := sp.remoteHeight - 1; i >= sp.nextHeight && i < math.MaxUint64; i-- {
			sp.RLock()
			bid, ok := sp.blks[i]
			sp.RUnlock()
			if !ok {
				// sync block from remote
				nbid, err := sp.GetTxBlockByHeight(i)
				if err != nil {
					logger.Debug("get block height fail: ", i, err)
					time.Sleep(5 * time.Second)
					break
				}
				bid = nbid
			} else {
				if i > 0 && i > sp.nextHeight {
					sp.RLock()
					// previous one exist, skip get?
					_, ok = sp.blks[i-1]
					sp.RUnlock()
					if ok {
						continue
					}
				}
			}

			blk, err := sp.GetTxBlock(bid)
			if err != nil {
				_, err := sp.GetTxBlockRemote(bid)
				if err != nil {
					logger.Debug("get block remote fail: ", i, bid, err)
					continue
				}
			} else {
				sp.Lock()
				sp.blks[blk.Height] = bid
				if blk.Height > 0 {
					sp.blks[blk.Height-1] = blk.PrevID
				}
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
			sp.RLock()
			bid, ok := sp.blks[i]
			sp.RUnlock()

			if !ok {
				logger.Debug("before process block, not have")
				break
			}

			sb, err := sp.GetTxBlock(bid)
			if err != nil {
				logger.Debugf("get tx block %d %s fail: %s", i, bid, err)
				break
			}

			if sb.Height != i {
				logger.Debugf("get tx block %d %s height not equal %d", i, bid, sb.Height)
				delete(sp.blks, i)
				break
			}

			err = sp.processTxBlock(sb)
			if err != nil {
				// clear all block above sp.nextHeight
				logger.Debug("process block fail: %s", err)

				sp.Lock()
				for j := i; j < sp.remoteHeight; j++ {
					delete(sp.blks, j)
				}
				sp.remoteHeight = i
				sp.Unlock()

				break
			}

			sp.Lock()
			delete(sp.blks, i)
			sp.nextHeight++
			sp.Unlock()
			stats.Record(sp.ctx, metrics.TxBlockSyncdHeight.M(int64(sp.nextHeight)))
		}

		sp.Lock()
		if sp.nextHeight > sp.remoteHeight {
			sp.remoteHeight = sp.nextHeight
		}
		sp.Unlock()
	}
}

func (sp *SyncPool) processTxBlock(sb *tx.SignedBlock) error {
	done := metrics.Timer(sp.ctx, metrics.TxBlockApply)
	defer done()

	bid := sb.Hash()
	logger.Debug("process block: ", sb.Height, bid)
	oRoot, err := sp.ApplyBlock(nil)
	if err != nil {
		return err
	}

	if !bytes.Equal(oRoot.Bytes(), sb.ParentRoot.Bytes()) {
		logger.Warnf("apply wrong state at height %d, got: %s, expected: %s", sb.Height, oRoot, sb.ParentRoot)
		sp.DeleteTxBlock(bid)
		sp.DeleteTxBlockHeight(sb.Height)

		sp.Lock()
		sp.bads[bid] = struct{}{}
		sp.retry++
		sp.Unlock()

		if sp.retry > 10 {
			panic("apply wrong state, should re-sync")
		}
		return xerrors.Errorf("apply wrong state, got: %s, expected: %s", oRoot, sb.ParentRoot)
	}

	newRoot, err := sp.ApplyBlock(sb)
	if err != nil {
		logger.Debug("apply block fail: ", err)
		return err
	}

	// todo: fix this
	if !bytes.Equal(newRoot.Bytes(), sb.Root.Bytes()) {
		logger.Warnf("apply has wrong state, got: %s, expected: %s", newRoot, sb.Root)
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

	bid := tb.Hash()

	sp.Lock()
	_, ok := sp.bads[bid]
	if ok {
		sp.Unlock()
		return nil
	}
	sp.Unlock()

	has, _ := sp.HasTxBlock(bid)
	if has {
		logger.Debug("add tx block, already have")
		return nil
	}

	stats.Record(sp.ctx, metrics.TxBlockReceived.M(1))

	if tb.GroupID != sp.groupID {
		return xerrors.Errorf("wrong block, group expected %d, got %d", sp.groupID, tb.GroupID)
	}

	// verify signaturs len >= threshold
	if tb.Height > 0 && tb.Len() < sp.thre {
		return xerrors.Errorf("block has not enough signers")
	}

	// verify
	ok, err := sp.RoleVerifyMulti(sp.ctx, hs.CalcHash(bid.Bytes(), hs.PhaseCommit), tb.MultiSignature)
	if err != nil {
		// for test
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
			// for test
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

	logger.Debug("add block ok: ", tb.Height, sp.nextHeight, sp.remoteHeight)

	sp.Lock()
	if tb.Height >= sp.nextHeight {
		sp.blks[tb.Height] = bid
		if tb.Height > 0 {
			sp.blks[tb.Height-1] = tb.PrevID
		}
	}

	if tb.Height >= sp.remoteHeight {
		sp.remoteHeight = tb.Height + 1
	}
	sp.Unlock()

	stats.Record(sp.ctx, metrics.TxBlockRemoteHeight.M(int64(sp.remoteHeight)))

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
