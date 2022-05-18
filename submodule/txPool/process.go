package txPool

import (
	"context"
	"sync"
	"time"

	"go.opencensus.io/stats"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/submodule/metrics"
)

// add: when >= nonce
type msgSet struct {
	nextDelete uint64
	info       map[uint64]*tx.SignedMessage // key: nonce
}

type InPool struct {
	lk sync.RWMutex

	*SyncPool

	ctx context.Context

	minerID uint64

	pending map[uint64]*msgSet // key: from; all currently processable tx

	msgChan chan *tx.SignedMessage // received msg from push pool

	blkDone chan *blkDigest
}

func NewInPool(ctx context.Context, localID uint64, sp *SyncPool) *InPool {
	pl := &InPool{
		SyncPool: sp,

		ctx:     ctx,
		minerID: localID,
		pending: make(map[uint64]*msgSet),
		msgChan: sp.msgChan,
		blkDone: sp.blkDone,
	}

	return pl
}

func (mp *InPool) Start() {
	go mp.sync()

	// load latest block and publish if exist
	// due to exit at not applying latest block
	si, err := mp.SyncGetInfo(mp.ctx)
	if err != nil {
		return
	}

	if si.SyncedHeight > 0 {
		// rebroadcast lastest
		bid, err := mp.GetTxBlockByHeight(si.SyncedHeight - 1)
		if err == nil {
			sb, err := mp.GetTxBlock(bid)
			if err == nil {
				mp.OnViewDone(sb)
			}
		}
	}

	bid, err := mp.GetTxBlockByHeight(si.SyncedHeight)
	if err == nil {
		sb, err := mp.GetTxBlock(bid)
		if err == nil {
			mp.OnViewDone(sb)
		}
	}

	// enable inprocess callback
	mp.SyncPool.inProcess = true
}

func (mp *InPool) sync() {
	for {
		select {
		case <-mp.ctx.Done():
			logger.Debug("process block done")
			return
		case m := <-mp.msgChan:
			id := m.Hash()
			logger.Debug("add tx message: ", id, m.From, m.Nonce, m.Method)

			mp.lk.Lock()
			ms, ok := mp.pending[m.From]
			if !ok {
				ms = &msgSet{
					nextDelete: mp.StateGetNonce(mp.ctx, m.From),
					info:       make(map[uint64]*tx.SignedMessage),
				}

				mp.pending[m.From] = ms
			}

			// not add low nonce
			if ms.nextDelete <= m.Nonce {
				ms.info[m.Nonce] = m
			} else {
				logger.Debug("add tx msg fail: ", id, m.From, m.Nonce, m.Method, ms.nextDelete)
			}

			mp.lk.Unlock()
		case bh := <-mp.blkDone:
			logger.Debug("process new block at:", bh.height)

			mp.lk.Lock()
			for _, md := range bh.msgs {
				ms, ok := mp.pending[md.From]
				if ok {
					if ms.nextDelete != md.Nonce {
						logger.Debug("block delete message at: ", md.From, md.Nonce)
					}

					ms.nextDelete = md.Nonce + 1
					delete(ms.info, md.Nonce)
				}
			}
			mp.lk.Unlock()
		}
	}
}

func (mp *InPool) AddTxMsg(ctx context.Context, m *tx.SignedMessage) error {
	stats.Record(ctx, metrics.TxMessageReceived.M(1))

	err := mp.SyncPool.AddTxMsg(mp.ctx, m)
	if err != nil {
		stats.Record(ctx, metrics.TxMessageFailure.M(1))
		logger.Debug("add tx msg fails: ", err)
		return err
	}
	stats.Record(ctx, metrics.TxMessageSuccess.M(1))

	mp.msgChan <- m

	return nil
}

func (mp *InPool) CreateBlockHeader() (tx.RawHeader, error) {
	nrh := tx.RawHeader{
		GroupID: mp.groupID,
	}

	// synced; should get from state
	si, err := mp.SyncGetInfo(mp.ctx)
	if err != nil {
		return nrh, err
	}

	if si.SyncedHeight < si.RemoteHeight {
		return nrh, xerrors.Errorf("sync height expected %d, got %d", si.RemoteHeight, si.SyncedHeight)
	}

	sgi, err := mp.StateGetInfo(mp.ctx)
	if err != nil {
		return nrh, err
	}

	if sgi.Height != si.SyncedHeight {
		logger.Debug("create block state height is not equal")
		return nrh, xerrors.Errorf("create block state height is not equal")
	}

	nt := time.Now().Unix()
	slot := uint64(nt-build.BaseTime) / build.SlotDuration
	if sgi.Slot >= slot {
		return nrh, xerrors.Errorf("create new block time is not up, skipped, now: %d, expected large than %d", slot, sgi.Slot)
	}

	logger.Debugf("create block header version %d at height %d, slot: %d", sgi.Version, si.SyncedHeight, slot)

	nrh.Version = sgi.Version
	nrh.Height = sgi.Height
	nrh.Slot = slot
	nrh.PrevID = sgi.BlockID
	nrh.MinerID = mp.GetLeader(slot)

	return nrh, nil
}

func (mp *InPool) Propose(rh tx.RawHeader) (tx.MsgSet, error) {

	logger.Debugf("create block propose at height %d", rh.Height)
	mSet := tx.MsgSet{
		Msgs: make([]tx.SignedMessage, 0, 16),
	}

	nt := time.Now()

	// reset
	oldRoot, err := mp.ValidateBlock(nil)
	if err != nil {
		return mSet, err
	}

	sb := &tx.SignedBlock{
		RawBlock: tx.RawBlock{
			RawHeader: rh,
		},
	}

	newRoot, err := mp.ValidateBlock(sb)
	if err != nil {
		return mSet, err
	}

	mSet.ParentRoot = oldRoot
	mSet.Root = newRoot

	mp.lk.RLock()
	defer mp.lk.RUnlock()

	// todo: block 0 is special
	if sb.Height == 0 {
		cnt := 0
		for from, ms := range mp.pending {
			nc := mp.StateGetNonce(mp.ctx, from)
			for i := nc; ; i++ {
				m, ok := ms.info[i]
				if ok {
					if m.Method != tx.AddRole {
						break
					}

					ri, err := mp.RoleGet(mp.ctx, m.From)
					if err != nil {
						break
					}

					if ri.Type != pb.RoleInfo_Keeper {
						break
					}

					// validate message
					tr := tx.Receipt{
						Err: 0,
					}
					nroot, err := mp.ValidateMsg(&m.Message)
					if err != nil {
						logger.Debug("block message invalid:", m.From, m.Nonce, m.Method, err)
						tr.Err = 1
						tr.Extra = err.Error()
					} else {
						cnt++
					}

					mSet.Root = nroot

					mSet.Msgs = append(mSet.Msgs, *m)
					mSet.Receipts = append(mSet.Receipts, tr)
				} else {
					break
				}
			}
		}
		if cnt < mp.GetQuorumSize() {
			return mSet, xerrors.Errorf("not have enough keepers, got %d expect %d", cnt, mp.GetQuorumSize())
		}
		return mSet, nil
	}

	// block size should be less than 1MiB
	msgCnt := 0
	rLen := 0
	for from, ms := range mp.pending {
		// should load from memory?
		nc := mp.StateGetNonce(mp.ctx, from)
		for i := nc; ; i++ {
			m, ok := ms.info[i]
			if ok {
				if rLen+len(m.Params) > 1_000_000 {
					break
				}

				// validate message
				tr := tx.Receipt{
					Err: 0,
				}
				nroot, err := mp.ValidateMsg(&m.Message)
				if err != nil {
					logger.Debug("block message invalid:", m.From, m.Nonce, m.Method, err)
					tr.Err = 1
					tr.Extra = err.Error()
				}

				mSet.Root = nroot

				mSet.Msgs = append(mSet.Msgs, *m)
				mSet.Receipts = append(mSet.Receipts, tr)
				msgCnt++
				rLen += len(m.Params)

				if time.Since(nt).Seconds() > 10 {
					break
				}
			} else {
				break
			}
		}

		if rLen > 1_000_000 {
			break
		}

		if time.Since(nt).Seconds() > 10 {
			break
		}
	}

	logger.Debugf("create block propose at height %d, msgCnt %d, msgLen %d, cost %s", rh.Height, msgCnt, rLen, time.Since(nt))

	return mSet, nil
}

func (mp *InPool) OnPropose(sb *tx.SignedBlock) error {
	logger.Debugf("create block OnPropose at height %d", sb.Height)

	nt := time.Now()
	oRoot, err := mp.ValidateBlock(nil)
	if err != nil {
		return err
	}

	if !oRoot.Equal(sb.ParentRoot) {
		logger.Debugf("OnPropose at %d has wrong state, got: %s, expected: %s", sb.Height, oRoot, sb.ParentRoot)
		return xerrors.Errorf("OnPropose at %d wrong state, got: %s, expected: %s", sb.Height, oRoot, sb.ParentRoot)
	}

	newRoot, err := mp.ValidateBlock(sb)
	if err != nil {
		return err
	}

	for i, sm := range sb.Msgs {
		// validate msg sign
		ok, err := mp.RoleVerify(mp.ctx, sm.From, sm.Hash().Bytes(), sm.Signature)
		if err != nil {
			logger.Debugf("OnPropose at %d has wrong message", sb.Height)
			return err
		}

		if !ok {
			return xerrors.Errorf("invalid sign for: %d, %d %d", sm.From, sm.Nonce, sm.Method)
		}

		// validate message
		newRoot, err = mp.ValidateMsg(&sm.Message)
		if err != nil {
			// should not; todo
			if sb.Receipts[i].Err == 0 {
				logger.Debug("fail to validate message, shoule be right: ", newRoot, err)
				return xerrors.Errorf("fail to validate message, shoule be right")
			}
		} else {
			if sb.Receipts[i].Err != 0 {
				logger.Debug("fail to validate message, shoule be wrong: ", newRoot)
				return xerrors.Errorf("fail to validate message, shoule be wrong")
			}
		}
	}

	// todo: should handle this
	if !newRoot.Equal(sb.Root) {
		logger.Debugf("OnPropose has wrong state at height %d, got: %s, expected: %s", sb.Height, newRoot, sb.Root)
		return xerrors.Errorf("OnPropose has wrong state at height %d, got: %s, expected: %s", sb.Height, newRoot, sb.Root)
	}

	logger.Debugf("create block OnPropose at height %d cost %s", sb.Height, time.Since(nt))

	return nil
}

func (mp *InPool) OnViewDone(tb *tx.SignedBlock) error {
	logger.Debugf("create block OnViewDone at height %d", tb.Height)

	// add to local first
	err := mp.SyncPool.AddTxBlock(tb)
	if err != nil {
		return err
	}

	stats.Record(mp.ctx, metrics.TxBlockPublished.M(1))

	if tb.MinerID == mp.minerID {
		logger.Debugf("create new block at height: %d, slot: %d, now: %s, prev: %s, state now: %s, parent: %s, has message: %d", tb.Height, tb.Slot, tb.Hash().String(), tb.PrevID.String(), tb.Root.String(), tb.ParentRoot.String(), len(tb.Msgs))
	}

	return mp.INetService.PublishTxBlock(mp.ctx, tb)
}

func (mp *InPool) GetMembers() []uint64 {
	return mp.StateGetAllKeepers(mp.ctx)
}

func (mp *InPool) GetLeader(slot uint64) uint64 {
	mems := mp.StateGetAllKeepers(mp.ctx)
	if len(mems) > 0 {
		return mems[slot%uint64(len(mems))]
	} else {
		return mp.minerID
	}
}

// quorum size
func (mp *InPool) GetQuorumSize() int {
	return mp.GetThreshold(mp.ctx)
}
