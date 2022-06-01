package txPool

import (
	"context"
	"math"
	"sync"
	"time"

	"go.opencensus.io/stats"
	"golang.org/x/sync/semaphore"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/api"
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

var _ api.IChainSync = &SyncPool{}

type SyncPool struct {
	lk sync.RWMutex

	tx.Store

	api.INetService

	*state.StateMgr

	ctx context.Context

	thre         int
	groupID      uint64
	nextHeight   uint64 // next synced
	remoteHeight uint64 // next remote

	blks map[uint64]types.MsgID // key: height

	ready bool

	msgDone chan *blkDigest
	inPush  bool

	msgChan   chan *tx.SignedMessage
	blkDone   chan *blkDigest
	inProcess bool
}

// sync
func NewSyncPool(ctx context.Context, groupID uint64, thre int, st *state.StateMgr, ts tx.Store, ins api.INetService) *SyncPool {
	sp := &SyncPool{
		INetService: ins,
		Store:       ts,

		StateMgr: st,

		ctx: ctx,

		thre:         thre,
		groupID:      groupID,
		nextHeight:   0,
		remoteHeight: 0,

		blks: make(map[uint64]types.MsgID),

		msgDone: make(chan *blkDigest, 64),

		msgChan: make(chan *tx.SignedMessage, 128),
		blkDone: make(chan *blkDigest, 64),
	}

	sp.load()

	return sp
}

func (sp *SyncPool) Start() {
	logger.Debug("start sync pool")
	go sp.syncBlock()
	go sp.handleBlock()
}

func (sp *SyncPool) SetReady() {
	sp.lk.Lock()
	sp.ready = true
	sp.lk.Unlock()
}

func (sp *SyncPool) load() {
	// todo: handle case if msglen > 0
	ht := sp.GetHeight(sp.ctx)
	sp.nextHeight = ht
	sp.remoteHeight = ht

	logger.Debug("block synced to: ", sp.nextHeight)
}

// far behind
func (sp *SyncPool) syncBlock() {
	tc := time.NewTicker(10 * time.Second)
	defer tc.Stop()

	for {
		select {
		case <-sp.ctx.Done():
			return
		case <-tc.C:
		}

		if sp.remoteHeight == sp.nextHeight {
			logger.Debug("regular get block synced at:", sp.nextHeight)
			continue
		}

		logger.Debug("regular get block:", sp.nextHeight, sp.remoteHeight)

		// if far from remote, parallel get it
		if sp.remoteHeight > sp.nextHeight+128 {
			var wg sync.WaitGroup
			sm := semaphore.NewWeighted(128)
			for i := sp.nextHeight; i < sp.remoteHeight; i++ {
				err := sm.Acquire(sp.ctx, 1)
				if err != nil {
					break
				}

				wg.Add(1)
				go func(ht uint64) {
					defer sm.Release(1)
					defer wg.Done()
					sp.getTxBlockByHeight(ht)
				}(i)
			}
			wg.Wait()
		}

	}
}

func (sp *SyncPool) handleBlock() {
	tc := time.NewTicker(3 * time.Second)
	defer tc.Stop()

	for {
		select {
		case <-sp.ctx.Done():
			return
		case <-tc.C:
		}

		if sp.remoteHeight == sp.nextHeight {
			logger.Debug("regular process block synced at:", sp.nextHeight)
			continue
		}

		logger.Debug("regular process block get:", sp.nextHeight, sp.remoteHeight)
		// sync all block from end -> begin
		// use prevID to find
		for i := sp.remoteHeight - 1; i >= sp.nextHeight && i < math.MaxUint64; i-- {
			sp.lk.RLock()
			bid, ok := sp.blks[i]
			sp.lk.RUnlock()
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
					// previous one exist, skip get?
					sp.lk.RLock()
					_, ok = sp.blks[i-1]
					sp.lk.RUnlock()
					if ok {
						continue
					}
				}
			}

			blk, err := sp.GetTxBlock(bid)
			if err != nil {
				_, err := sp.getTxBlockRemote(bid)
				if err != nil {
					logger.Debug("get block remote fail: ", i, bid, err)
				}
				continue
			}

			sp.lk.Lock()
			sp.blks[blk.Height] = bid
			if blk.Height > 0 {
				sp.blks[blk.Height-1] = blk.PrevID
			}
			sp.lk.Unlock()
		}

		logger.Debug("regular process block:", sp.nextHeight, sp.remoteHeight)

		// process syncd blk
		for i := sp.nextHeight; i < sp.remoteHeight; i++ {
			sp.lk.RLock()
			bid, ok := sp.blks[i]
			sp.lk.RUnlock()
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
				sp.lk.Lock()
				delete(sp.blks, i)
				sp.lk.Unlock()
				break
			}

			err = sp.processTxBlock(sb)
			if err != nil {
				// clear all block above sp.nextHeight
				logger.Debugf("process tx block fail: %s", err)

				sp.lk.Lock()
				for j := i; j < sp.remoteHeight; j++ {
					delete(sp.blks, j)
				}
				sp.remoteHeight = i
				sp.lk.Unlock()
				break
			}

			sp.lk.Lock()
			delete(sp.blks, i)
			sp.nextHeight++
			sp.lk.Unlock()
			stats.Record(sp.ctx, metrics.TxBlockSyncdHeight.M(int64(sp.nextHeight)))
		}

		sp.lk.Lock()
		if sp.nextHeight > sp.remoteHeight {
			sp.remoteHeight = sp.nextHeight
		}
		sp.lk.Unlock()
	}
}

func (sp *SyncPool) processTxBlock(sb *tx.SignedBlock) error {
	done := metrics.Timer(sp.ctx, metrics.TxBlockApply)
	defer done()

	bid := sb.Hash()
	logger.Debug("process tx block: ", sb.Height, bid)

	newRoot, err := sp.ApplyBlock(sb)
	if err != nil {
		logger.Warnf("apply wrong state at height %d, err: %s", sb.Height, err)
		if !newRoot.Equal(sb.Root) {
			// delete wrong block
			sp.DeleteTxBlock(bid)
			sp.DeleteTxBlockHeight(sb.Height)
		}
		panic("check your storage space left and re-sync")
	}

	mds := &blkDigest{
		height: sb.Height,
		msgs:   make([]tx.MessageDigest, 0, len(sb.Msgs)),
	}

	for i, msg := range sb.Msgs {
		ms := &tx.MsgState{
			BlockID: bid,
			Height:  sb.Height,
			Status:  sb.Receipts[i],
		}

		mid := msg.Hash()

		sp.PutTxMsgState(mid, ms)

		md := tx.MessageDigest{
			ID:    mid,
			From:  msg.From,
			Nonce: msg.Nonce,
		}

		mds.msgs = append(mds.msgs, md)
	}

	if sp.inPush {
		sp.msgDone <- mds
	}

	if sp.inProcess {
		sp.blkDone <- mds
	}

	logger.Debugf("process tx block '%s' done, current: %d, remote: %d", bid, sb.Height, sp.remoteHeight)

	return nil
}

func (sp *SyncPool) GetSyncStatus(ctx context.Context) bool {
	sp.lk.RLock()
	defer sp.lk.RUnlock()
	if sp.nextHeight == sp.remoteHeight && sp.ready {
		return true
	}
	return false
}

func (sp *SyncPool) SyncGetInfo(ctx context.Context) (*api.SyncInfo, error) {
	sp.lk.RLock()
	defer sp.lk.RUnlock()
	si := &api.SyncInfo{
		Status:       sp.ready,
		SyncedHeight: sp.nextHeight,
		RemoteHeight: sp.remoteHeight,
	}

	return si, nil
}

func (sp *SyncPool) SyncGetTxMsgStatus(ctx context.Context, mid types.MsgID) (*tx.MsgState, error) {
	return sp.Store.GetTxMsgState(mid)
}

func (sp *SyncPool) AddTxBlock(tb *tx.SignedBlock) error {
	logger.Debug("add block: ", tb.Height, sp.nextHeight, sp.remoteHeight)

	if tb.Height < sp.nextHeight {
		return xerrors.Errorf("height expected %d, got %d", sp.nextHeight, tb.Height)
	}

	sp.SetReady()

	bid := tb.Hash()
	has, _ := sp.HasTxBlock(bid)
	if has {
		logger.Debug("add block, already have")
		sp.lk.Lock()
		if tb.Height >= sp.remoteHeight {
			sp.remoteHeight = tb.Height + 1
		}
		sp.lk.Unlock()
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

	// verify block
	ok, err := sp.RoleVerifyMulti(sp.ctx, hs.CalcHash(bid.Bytes(), hs.PhaseCommit), tb.MultiSignature)
	if err != nil {
		return err
	}
	if !ok {
		return xerrors.Errorf("%s block at height %d sign is invalid", bid, tb.Height)
	}

	// verify all msgs
	for _, msg := range tb.Msgs {
		ok, err := sp.RoleVerify(sp.ctx, msg.From, msg.Hash().Bytes(), msg.Signature)
		if err != nil {
			return err
		}

		if !ok {
			return xerrors.Errorf("%s block at height %d msg %d sign is invalid", bid, tb.Height, msg.From)
		}
	}

	// store local
	bLen, err := sp.PutTxBlock(tb)
	if err != nil {
		logger.Debug("add block: ", err)
		return err
	}

	stats.Record(context.TODO(),
		metrics.TxBlockBytes.M(int64(bLen)),
	)

	logger.Debug("add block ok: ", tb.Height, sp.nextHeight, sp.remoteHeight)

	sp.lk.Lock()
	if tb.Height >= sp.nextHeight {
		sp.blks[tb.Height] = bid
		if tb.Height > sp.nextHeight {
			sp.blks[tb.Height-1] = tb.PrevID
		}
	}
	if tb.Height >= sp.remoteHeight {
		sp.remoteHeight = tb.Height + 1
	}
	sp.lk.Unlock()

	stats.Record(sp.ctx, metrics.TxBlockRemoteHeight.M(int64(sp.remoteHeight)))

	return nil
}

// over network
func (sp *SyncPool) getTxBlockRemote(bid types.MsgID) (*tx.SignedBlock, error) {
	// fetch it over network
	key := store.NewKey("tx", pb.MetaType_TX_BlockKey, bid.String())
	//key := store.NewKey(pb.MetaType_TX_BlockKey, bid.String())
	res, err := sp.INetService.Fetch(sp.ctx, key)
	if err != nil {
		return nil, err
	}
	tb := new(tx.SignedBlock)
	err = tb.Deserialize(res)
	if err != nil {
		return nil, err
	}

	logger.Debugf("get block id from remote %s", bid)

	return tb, sp.AddTxBlock(tb)
}

func (sp *SyncPool) getTxBlockByHeight(ht uint64) {
	bid, err := sp.GetTxBlockByHeight(ht)
	if err != nil {
		// fetch it over network
		key := store.NewKey("tx", pb.MetaType_Tx_BlockHeightKey, ht)
		//key := store.NewKey(pb.MetaType_Tx_BlockHeightKey, ht)
		res, err := sp.INetService.Fetch(sp.ctx, key)
		if err != nil {
			return
		}

		bid, err = types.FromBytes(res)
		if err != nil {
			logger.Debug("get block id at height fail: ", err)
			return
		}

		logger.Debugf("get block id %s at height %d", bid, ht)
	}

	ok, err := sp.HasTxBlock(bid)
	if err != nil || !ok {
		sp.getTxBlockRemote(bid)
	}
}

func (sp *SyncPool) AddTxMsg(ctx context.Context, msg *tx.SignedMessage) error {
	nonce := sp.StateGetNonce(sp.ctx, msg.From)
	if msg.Nonce < nonce {
		return xerrors.Errorf("%d nonce expected no less than %d, got %d", msg.From, nonce, msg.Nonce)
	}

	mid := msg.Hash()
	ok, err := sp.HasTxMsg(mid)
	if err != nil || !ok {
		// need valid its content with settle chain
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

		// need store?
		return sp.PutTxMsg(msg, false)
	}
	return nil
}
