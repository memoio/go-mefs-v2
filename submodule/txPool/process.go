package txPool

import (
	"context"
	"sync"
	"time"

	msign "github.com/memoio/go-mefs-v2/lib/multiSign"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
)

type msgSet struct {
	min  uint64
	max  uint64
	info map[uint64]*tx.MessageDigest // key: nonce
}

type InPool struct {
	sync.Mutex

	*SyncPool

	ctx context.Context

	nextHeight uint64
	curBlockID types.MsgID

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

	return pl
}

func (mp *InPool) sync() {
	tc := time.NewTicker(30 * time.Second)
	defer tc.Stop()

	for {
		select {
		case <-mp.ctx.Done():
			return
		case <-tc.C:
			blk := mp.CreateBlock()

			id, _ := blk.Hash()

			sig, _ := mp.RoleSign(mp.ctx, id.Bytes(), types.SigSecp256k1)

			tb := &tx.Block{
				BlockHeader:    blk,
				MultiSignature: msign.NewMultiSignature(types.SigSecp256k1),
			}

			tb.MultiSignature.Add(mp.localID, sig)

			logger.Debug("create new block at height:", tb.Height)

			mp.INetService.PublishTxBlock(mp.ctx, tb)

			mp.AddTxBlock(tb)

		case bh := <-mp.blkDone:
			mp.Lock()
			logger.Debug("process new block:", bh.Height, mp.nextHeight)
			bid, _ := bh.Hash()
			mp.curBlockID = bid

			for _, md := range bh.Txs {
				ms, ok := mp.pending[md.From]
				if !ok {
					ms = &msgSet{
						min:  md.Nonce + 1,
						max:  md.Nonce + 1,
						info: make(map[uint64]*tx.MessageDigest),
					}

					mp.pending[md.From] = ms
				}
				if ms.min < md.Nonce+1 {
					ms.min = md.Nonce + 1
				}

				if ms.max < ms.min {
					ms.max = ms.min
				}

				delete(ms.info, md.Nonce)
			}
			mp.nextHeight = bh.Height + 1
			mp.Unlock()
		}
	}
}

func (mp *InPool) AddMsg(m *tx.SignedMessage) error {
	nonce := mp.SyncPool.GetNextNonce(m.From)
	if m.Nonce < nonce {
		return ErrLowNonce
	}

	err := mp.SyncPool.AddTxMsg(m)
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
			min:  nonce,
			max:  nonce,
			info: make(map[uint64]*tx.MessageDigest),
		}

		mp.pending[m.From] = ms
	}

	if m.Nonce >= ms.max {
		ms.max = m.Nonce + 1
	}

	ms.info[m.Nonce] = md

	mp.Unlock()

	return nil
}

func (mp *InPool) CreateBlock() tx.BlockHeader {
	mp.Lock()
	defer mp.Unlock()

	nbh := tx.BlockHeader{
		Version: 1,
		Height:  mp.nextHeight,
		MinerID: mp.localID,
		PrevID:  mp.curBlockID,
		Time:    time.Now(),
		Txs:     make([]tx.MessageDigest, 0, 16),
	}

	for _, ms := range mp.pending {
		for i := ms.min; i < ms.max; i++ {
			md, ok := ms.info[i]
			if ok {
				// validate message
				nbh.Txs = append(nbh.Txs, *md)
				ms.min++
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
