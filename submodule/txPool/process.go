package txPool

import (
	"context"
	"sync"
	"time"

	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
)

// add: when >= nonce
type msgSet struct {
	nextDelete uint64
	info       map[uint64]*tx.MessageDigest // key: nonce
}

type InPool struct {
	sync.Mutex

	*SyncPool

	ctx context.Context

	pending map[uint64]*msgSet // key: from; all currently processable tx

	blkDone chan *tx.BlockHeader
}

func NewInPool(ctx context.Context, sp *SyncPool) *InPool {
	pl := &InPool{
		ctx:      ctx,
		SyncPool: sp,
		pending:  make(map[uint64]*msgSet),
		blkDone:  sp.blkDone,
	}

	go pl.sync()

	// enable inprocess callback
	sp.inProcess = true

	return pl
}

func (mp *InPool) sync() {
	tc := time.NewTicker(30 * time.Second)
	defer tc.Stop()

	for {
		select {
		case <-mp.ctx.Done():
			logger.Debug("process block done")
			return
		case <-tc.C:
			blk := mp.createBlock()

			id, _ := blk.Hash()

			sig, _ := mp.RoleSign(mp.ctx, id.Bytes(), types.SigSecp256k1)

			tb := &tx.Block{
				BlockHeader:    blk,
				MultiSignature: types.NewMultiSignature(types.SigSecp256k1),
			}

			tb.MultiSignature.Add(mp.localID, sig)

			logger.Debugf("create new block at height: %d, now: %s, prev: %s, has msg: %d", tb.Height, id.String(), blk.PrevID.String(), len(blk.Txs))

			mp.INetService.PublishTxBlock(mp.ctx, tb)

			mp.AddTxBlock(tb)

		case bh := <-mp.blkDone:
			logger.Debug("process new block:", bh.Height)

			mp.Lock()
			for _, md := range bh.Txs {
				ms, ok := mp.pending[md.From]
				if !ok {
					ms = &msgSet{
						nextDelete: md.Nonce,
						info:       make(map[uint64]*tx.MessageDigest),
					}

					mp.pending[md.From] = ms
				}

				if ms.nextDelete != md.Nonce {
					logger.Debug("block delete msg at: ", md.From, md.Nonce)
				}

				ms.nextDelete = md.Nonce + 1

				delete(ms.info, md.Nonce)
			}
			mp.Unlock()
		}
	}
}

func (mp *InPool) AddTxMsg(ctx context.Context, m *tx.SignedMessage) error {
	nonce := mp.SyncPool.GetNonce(m.From)
	if m.Nonce < nonce {
		return ErrLowNonce
	}

	err := mp.SyncPool.AddTxMsg(mp.ctx, m)
	if err != nil {
		return err
	}

	id, err := m.Hash()
	if err != nil {
		return err
	}

	md := &tx.MessageDigest{
		ID:    id,
		From:  m.From,
		Nonce: m.Nonce,
	}

	mp.Lock()
	ms, ok := mp.pending[m.From]
	if !ok {
		ms = &msgSet{
			nextDelete: nonce,
			info:       make(map[uint64]*tx.MessageDigest),
		}

		mp.pending[m.From] = ms
	}

	ms.info[m.Nonce] = md

	mp.Unlock()

	return nil
}

func (mp *InPool) createBlock() tx.BlockHeader {
	mp.Lock()
	defer mp.Unlock()

	// synced
	_, rh := mp.GetSyncHeight()

	bid, _ := mp.GetTxBlockByHeight(rh - 1)

	nbh := tx.BlockHeader{
		Version: 1,
		Height:  rh,
		MinerID: mp.localID,
		PrevID:  bid,
		Time:    time.Now(),
		Txs:     make([]tx.MessageDigest, 0, 16),
	}

	for from, ms := range mp.pending {
		nc := mp.GetNonce(from)
		for i := nc; ; i++ {
			md, ok := ms.info[i]
			if ok {
				// validate message
				tr := tx.Receipt{
					Err: 0,
				}
				nbh.Txs = append(nbh.Txs, *md)
				nbh.Receipts = append(nbh.Receipts, tr)
			} else {
				break
			}
		}
	}

	return nbh
}

func (mp *InPool) PushBlock(m *tx.Message) bool {

	return false
}
