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

	pending map[uint64]*msgSet // key: from; all currently processable tx

	msgChan chan *tx.SignedMessage

	blkDone chan *blkDigest
}

func NewInPool(ctx context.Context, sp *SyncPool) *InPool {
	pl := &InPool{
		ctx:      ctx,
		SyncPool: sp,
		pending:  make(map[uint64]*msgSet),
		msgChan:  sp.msgChan,
		blkDone:  sp.blkDone,
	}

	return pl
}

func (mp *InPool) Start() {
	go mp.sync()

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
					nextDelete: m.Nonce,
					info:       make(map[uint64]*tx.SignedMessage),
				}

				mp.pending[m.From] = ms
			}
			ms.info[m.Nonce] = m
			mp.lk.Unlock()
		case bh := <-mp.blkDone:
			logger.Debug("process new block at:", bh.height)

			mp.lk.Lock()
			for _, md := range bh.msgs {
				ms, ok := mp.pending[md.From]
				if !ok {
					ms = &msgSet{
						nextDelete: md.Nonce,
						info:       make(map[uint64]*tx.SignedMessage),
					}

					mp.pending[md.From] = ms
				}

				if ms.nextDelete != md.Nonce {
					logger.Debug("block delete message at: ", md.From, md.Nonce)
				}

				ms.nextDelete = md.Nonce + 1

				delete(ms.info, md.Nonce)
			}
			mp.lk.Unlock()
		}
	}
}

func (mp *InPool) AddTxMsg(ctx context.Context, m *tx.SignedMessage) error {
	nonce := mp.SyncPool.GetNonce(mp.ctx, m.From)
	if m.Nonce < nonce {
		return xerrors.Errorf("nonce expected no less than %d, got %d", nonce, m.Nonce)
	}

	stats.Record(ctx, metrics.TxMessageReceived.M(1))

	err := mp.SyncPool.AddTxMsg(mp.ctx, m)
	if err != nil {
		logger.Debug("add tx msg fails: ", err)
		return err
	}

	// need valid its content with settle chain
	stats.Record(ctx, metrics.TxMessageSuccess.M(1))

	mp.msgChan <- m

	return nil
}

func (mp *InPool) CreateBlockHeader() (tx.RawHeader, error) {
	nrh := tx.RawHeader{
		Version: 1,
		GroupID: mp.groupID,
	}

	// synced; should get from state
	lh, rh := mp.GetSyncHeight(mp.ctx)
	if lh < rh {
		return nrh, xerrors.Errorf("sync height expected %d, got %d", rh, lh)
	}

	appliedHeight := mp.GetHeight(mp.ctx)
	if appliedHeight != lh {
		logger.Debug("create block state height is not equal")
	}

	nt := time.Now().Unix()
	slot := uint64(nt-build.BaseTime) / build.SlotDuration
	appliedSlot := mp.GetSlot(mp.ctx)
	if appliedSlot >= slot {
		return nrh, xerrors.Errorf("create new block time is not up, skipped, now: %d, expected large than %d", slot, appliedSlot)
	}

	logger.Debugf("create block header at height %d, slot: %d", rh, slot)

	bid, err := mp.GetTxBlockByHeight(rh - 1)
	if err != nil {
		return nrh, err
	}

	nrh.Height = rh
	nrh.Slot = slot
	nrh.PrevID = bid
	nrh.MinerID = mp.GetLeader(slot)

	return nrh, nil
}

func (mp *InPool) Propose(rh tx.RawHeader) (tx.MsgSet, error) {

	logger.Debugf("create block propose at height %d", rh.Height)
	msgSet := tx.MsgSet{
		Msgs: make([]tx.SignedMessage, 0, 16),
	}

	nt := time.Now()

	// reset
	oldRoot, err := mp.ValidateBlock(nil)
	if err != nil {
		return msgSet, err
	}

	sb := &tx.SignedBlock{
		RawBlock: tx.RawBlock{
			RawHeader: rh,
		},
	}

	newRoot, err := mp.ValidateBlock(sb)
	if err != nil {
		return msgSet, err
	}

	msgSet.ParentRoot = oldRoot
	msgSet.Root = newRoot

	mp.lk.RLock()
	defer mp.lk.RUnlock()

	// todo: block 0 is special
	if sb.Height == 0 {
		cnt := 0
		for from, ms := range mp.pending {
			nc := mp.GetNonce(mp.ctx, from)
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
						logger.Debug("block message invalid:", m.From, m.Nonce, err)
						tr.Err = 1
						tr.Extra = []byte(err.Error())
					} else {
						cnt++
					}

					msgSet.Root = nroot

					msgSet.Msgs = append(msgSet.Msgs, *m)
					msgSet.Receipts = append(msgSet.Receipts, tr)
				} else {
					break
				}
			}
		}
		if cnt < mp.GetQuorumSize() {
			return msgSet, xerrors.Errorf("not have enough keepers")
		}
		return msgSet, nil
	}

	// block size should be less than 1MiB
	msgCnt := 0
	rLen := 0
	for from, ms := range mp.pending {
		// should load from memory?
		nc := mp.GetNonce(mp.ctx, from)
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
					logger.Debug("block message invalid:", m.From, m.Nonce, err)
					tr.Err = 1
					tr.Extra = []byte(err.Error())
				}

				msgSet.Root = nroot

				msgSet.Msgs = append(msgSet.Msgs, *m)
				msgSet.Receipts = append(msgSet.Receipts, tr)
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

	logger.Debugf("create block propose at height %d, msgCnt %d, msgLen %d, cost %f", rh.Height, msgCnt, rLen, time.Since(nt).Seconds())

	return msgSet, nil
}

func (mp *InPool) OnPropose(sb *tx.SignedBlock) error {
	logger.Debugf("create block OnPropose at height %d", sb.Height)

	nt := time.Now()
	oRoot, err := mp.ValidateBlock(nil)
	if err != nil {
		return err
	}

	if !oRoot.Equal(sb.ParentRoot) {
		logger.Warnf("OnPropose at %d has wrong state, got: %s, expected: %s", sb.Height, oRoot, sb.ParentRoot)
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
			logger.Warnf("OnPropose at %d has wrong message", sb.Height)
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
				logger.Error("fail to validate message, shoule be right: ", newRoot, err)
				return xerrors.Errorf("fail to validate message, shoule be right")
			}
		} else {
			if sb.Receipts[i].Err != 0 {
				logger.Error("fail to validate message, shoule be wrong: ", newRoot)
				return xerrors.Errorf("fail to validate message, shoule be wrong")
			}
		}
	}

	// todo: should handle this
	if !newRoot.Equal(sb.Root) {
		logger.Warnf("OnPropose has wrong state at height %d, got: %s, expected: %s", sb.Height, newRoot, sb.Root)
		return xerrors.Errorf("OnPropose has wrong state at height %d, got: %s, expected: %s", sb.Height, newRoot, sb.Root)
	}

	logger.Debugf("create block OnPropose at height %d cost %d", sb.Height, time.Since(nt))

	return nil
}

func (mp *InPool) OnViewDone(tb *tx.SignedBlock) error {
	logger.Debugf("create block OnViewDone at height %d", tb.Height)

	stats.Record(mp.ctx, metrics.TxBlockPublished.M(1))
	mp.INetService.PublishTxBlock(mp.ctx, tb)

	if tb.MinerID == mp.localID {
		logger.Debugf("create new block at height: %d, slot: %d, now: %s, prev: %s, state now: %s, parent: %s, has message: %d", tb.Height, tb.Slot, tb.Hash().String(), tb.PrevID.String(), tb.Root.String(), tb.ParentRoot.String(), len(tb.Msgs))
	}

	return mp.SyncPool.AddTxBlock(tb)
}

func (mp *InPool) GetMembers() []uint64 {
	return mp.GetAllKeepers(mp.ctx)
}

func (mp *InPool) GetLeader(slot uint64) uint64 {
	mems := mp.GetAllKeepers(mp.ctx)
	if len(mems) > 0 {
		return mems[slot%uint64(len(mems))]
	} else {
		return mp.localID
	}
}

// quorum size
func (mp *InPool) GetQuorumSize() int {
	return mp.GetThreshold(mp.ctx)
}
