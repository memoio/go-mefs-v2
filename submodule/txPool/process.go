package txPool

import (
	"context"
	"sync"
	"time"

	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"golang.org/x/xerrors"
)

type mesWithID struct {
	mid types.MsgID
	mes *tx.Message
}

// add: when >= nonce
type msgSet struct {
	nextDelete uint64
	info       map[uint64]*mesWithID // key: nonce
}

type InPool struct {
	sync.Mutex

	*SyncPool

	ctx context.Context

	pending map[uint64]*msgSet // key: from; all currently processable tx

	msgChan chan *tx.Message

	blkDone chan *tx.BlockHeader
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
	tc := time.NewTicker(3 * time.Second)
	defer tc.Stop()

	for {
		select {
		case <-mp.ctx.Done():
			logger.Debug("process block done")
			return
		case m := <-mp.msgChan:
			id := m.Hash()

			logger.Debug("add tx message: ", id, m.From, m.Nonce, m.Method)

			md := &mesWithID{
				mid: id,
				mes: m,
			}

			mp.Lock()
			ms, ok := mp.pending[m.From]
			if !ok {
				ms = &msgSet{
					nextDelete: m.Nonce,
					info:       make(map[uint64]*mesWithID),
				}

				mp.pending[m.From] = ms
			}
			ms.info[m.Nonce] = md
			mp.Unlock()
		case <-tc.C:
			tb, err := mp.createBlock()
			if err != nil {
				logger.Debug("create block err: ", err)
				continue
			}

			id := tb.Hash()

			sig, err := mp.RoleSign(mp.ctx, mp.localID, id.Bytes(), types.SigSecp256k1)
			if err != nil {
				continue
			}

			tb.MultiSignature.Add(mp.localID, sig)

			logger.Debugf("create new block at height: %d, slot: %d, now: %s, prev: %s, state now: %s, parent: %s, has message: %d", tb.Height, tb.Slot, id.String(), tb.PrevID.String(), tb.Root.String(), tb.ParentRoot.String(), len(tb.Txs))

			mp.AddTxBlock(tb)

			mp.INetService.PublishTxBlock(mp.ctx, tb)

		case bh := <-mp.blkDone:
			logger.Debug("process new block:", bh.Height)

			mp.Lock()
			for _, md := range bh.Txs {
				ms, ok := mp.pending[md.From]
				if !ok {
					ms = &msgSet{
						nextDelete: md.Nonce,
						info:       make(map[uint64]*mesWithID),
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

	mp.msgChan <- &m.Message

	return nil
}

func (mp *InPool) createBlock() (*tx.Block, error) {
	mp.Lock()
	defer mp.Unlock()

	// synced; should get from state
	lh, rh := mp.GetSyncHeight(mp.ctx)
	if lh < rh {
		return nil, xerrors.Errorf("sync height expected %d, got %d", rh, lh)
	}

	bid, err := mp.GetTxBlockByHeight(rh - 1)
	if err != nil {
		return nil, err
	}

	appliedHeight := mp.GetHeight(mp.ctx)
	if appliedHeight != lh {
		logger.Debug("create block state height is not equal")
	}

	nt := time.Now().Unix()
	slot := uint64(nt-build.BaseTime) / build.SlotDuration
	appliedSlot := mp.GetSlot(mp.ctx)
	if appliedSlot >= slot {
		return nil, xerrors.Errorf("create new block time is not up, skipped, now: %d, expected large than %d", slot, appliedSlot)
	}

	// check epoch > latest epoch

	nbh := &tx.Block{
		BlockHeader: tx.BlockHeader{
			RawHeader: tx.RawHeader{
				Version: 1,
				Height:  rh,
				Slot:    slot,
				MinerID: mp.localID,
				PrevID:  bid,
				Time:    time.Now(),
			},

			Txs: make([]tx.MessageDigest, 0, 16),
		},
		MultiSignature: types.NewMultiSignature(types.SigSecp256k1),
	}

	oldRoot, err := mp.ValidateMsg(nil)
	if err != nil {
		return nbh, err
	}

	// reset
	newRoot, err := mp.ValidateBlock(nbh)
	if err != nil {
		return nbh, err
	}

	nbh.ParentRoot = oldRoot
	nbh.Root = newRoot
	for from, ms := range mp.pending {
		nc := mp.GetNonce(mp.ctx, from)
		for i := nc; ; i++ {
			m, ok := ms.info[i]
			if ok {
				// validate message
				tr := tx.Receipt{
					Err: 0,
				}
				nroot, err := mp.ValidateMsg(m.mes)
				if err != nil {
					logger.Debug("block message invalid:", m.mes.From, m.mes.Nonce, err)
					tr.Err = 1
					tr.Extra = []byte(err.Error())
				}

				nbh.Root = nroot

				md := tx.MessageDigest{
					ID:    m.mid,
					From:  m.mes.From,
					Nonce: m.mes.Nonce,
				}

				nbh.Txs = append(nbh.Txs, md)
				nbh.Receipts = append(nbh.Receipts, tr)
			} else {
				break
			}
		}
	}

	return nbh, nil
}
