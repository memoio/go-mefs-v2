package txPool

import (
	"context"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
)

type mesWithID struct {
	mid types.MsgID
	mes *tx.Message
}

// add: when >= nonce
type msgSet struct {
	nextDelete uint64
	info       map[uint64]*tx.SignedMessage // key: nonce
}

type InPool struct {
	sync.Mutex

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

			mp.Lock()
			ms, ok := mp.pending[m.From]
			if !ok {
				ms = &msgSet{
					nextDelete: m.Nonce,
					info:       make(map[uint64]*tx.SignedMessage),
				}

				mp.pending[m.From] = ms
			}
			ms.info[m.Nonce] = m
			mp.Unlock()
		case bh := <-mp.blkDone:
			logger.Debug("process new block after:", bh.height)

			mp.Lock()
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
			mp.Unlock()
		}
	}
}

func (mp *InPool) AddTxMsg(ctx context.Context, m *tx.SignedMessage) error {
	nonce := mp.SyncPool.GetNonce(mp.ctx, m.From)
	if m.Nonce < nonce {
		return xerrors.Errorf("nonce expected no less than %d, got %d", nonce, m.Nonce)
	}

	err := mp.SyncPool.AddTxMsg(mp.ctx, m)
	if err != nil {
		logger.Debug("add tx msg fails: ", err)
		return err
	}

	// need valid its content with settle chain

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
	mp.RLock()
	defer mp.RUnlock()

	logger.Debugf("create block propose at height %d", rh.Height)
	msgSet := tx.MsgSet{
		Msgs: make([]tx.SignedMessage, 0, 16),
	}

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

	for from, ms := range mp.pending {
		nc := mp.GetNonce(mp.ctx, from)
		for i := nc; ; i++ {
			m, ok := ms.info[i]
			if ok {
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
			} else {
				break
			}
		}
	}

	return msgSet, nil
}

func (mp *InPool) OnPropose(sb *tx.SignedBlock) error {
	logger.Debugf("create block OnPropose at height %d", sb.Height)
	oRoot, err := mp.ValidateBlock(nil)
	if err != nil {
		return err
	}

	if !oRoot.Equal(sb.ParentRoot) {
		logger.Warnf("OnPropose has wrong state, got: %s, expected: %s", oRoot, sb.ParentRoot)
		return xerrors.Errorf("OnPropose wrong state, got: %s, expected: %s", oRoot, sb.ParentRoot)
	}

	newRoot, err := mp.ValidateBlock(sb)
	if err != nil {
		return err
	}

	for i, sm := range sb.Msgs {
		// validate msg sign
		ok, err := mp.RoleVerify(mp.ctx, sm.From, sm.Hash().Bytes(), sm.Signature)
		if err != nil {
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
		logger.Warnf("OnPropose has wrong state, got: %s, expected: %s", newRoot, sb.Root)
	}

	return nil
}

func (mp *InPool) OnViewDone(tb *tx.SignedBlock) error {
	logger.Debugf("create block OnViewDone at height %d", tb.Height)
	if tb.Height >= 0 || tb.MinerID == mp.localID {
		mp.INetService.PublishTxBlock(mp.ctx, tb)
	}

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
