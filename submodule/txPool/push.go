package txPool

import (
	"context"
	"sync"
	"time"

	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
)

type PushPool struct {
	sync.RWMutex

	*SyncPool

	ctx context.Context

	msg          []*tx.Message
	pendingNonce uint64                    // pending
	pendingMsg   map[types.MsgID]time.Time // to push

	msgDone chan types.MsgID

	ready bool
}

func NewPushPool(ctx context.Context, sp *SyncPool) *PushPool {
	pp := &PushPool{
		SyncPool: sp,
		ctx:      ctx,

		pendingNonce: sp.GetNextNonce(sp.localID),
		pendingMsg:   make(map[types.MsgID]time.Time),

		msgDone: sp.msgDone,
		ready:   false,
	}

	return pp
}

func (pp *PushPool) Sync() {
	tc := time.NewTicker(30 * time.Second)
	defer tc.Stop()

	for {
		select {
		case <-pp.ctx.Done():
			return
		case mid := <-pp.msgDone:
			pp.Lock()
			delete(pp.pendingMsg, mid)
			pp.Unlock()
		case <-tc.C:
			pp.Lock()
			if pp.GetSyncStatus() && !pp.ready {
				logger.Debug("push pool is ready")
				pp.ready = true
				pp.pendingNonce = pp.GetNextNonce(pp.localID)
			}
			pp.Unlock()

			pp.Lock()
			for mid, ctime := range pp.pendingMsg {
				if time.Since(ctime) > 5*time.Minute {
					sm, err := pp.GetTxMsg(mid)
					if err != nil {
						continue
					}
					pp.PushSignedMessage(sm)
				}
			}
			pp.Unlock()
		default:
			pp.RLock()
			if !pp.ready || len(pp.msg) == 0 {
				pp.RUnlock()
				continue
			}
			mes := pp.msg[0]
			pp.RUnlock()
			err := pp.PushMessage(mes)
			if err != nil {
				continue
			}

			pp.Lock()
			pp.msg = pp.msg[1:]
			pp.Unlock()
		}
	}
}

func (pp *PushPool) PushMessage(mes *tx.Message) error {
	logger.Debug("add tx message pool to push pool")
	pp.Lock()
	if !pp.ready {
		pp.msg = append(pp.msg, mes)
		pp.Unlock()
		return nil
	}

	// get nonce
	mes.Nonce = pp.pendingNonce
	pp.pendingNonce++

	mid, err := mes.Hash()
	if err != nil {
		pp.Unlock()
		return err
	}

	// sign
	sig, err := pp.RoleSign(pp.ctx, mid.Bytes(), types.SigSecp256k1)
	if err != nil {
		pp.Unlock()
		return err
	}

	sm := &tx.SignedMessage{
		Message:   *mes,
		Signature: sig,
	}

	pp.pendingMsg[mid] = time.Now()
	pp.Unlock()

	return pp.PushSignedMessage(sm)
}

func (pp *PushPool) PushSignedMessage(sm *tx.SignedMessage) error {
	mid, err := sm.Hash()
	if err != nil {
		return err
	}

	err = pp.PutTxMsg(sm)
	if err != nil {
		return err
	}

	pp.Lock()
	pp.pendingMsg[mid] = time.Now()
	pp.Unlock()

	// push out immediately
	err = pp.INetService.PublishTxMsg(pp.ctx, sm)
	if err != nil {
		return err
	}

	return nil
}

func (pp *PushPool) ReplaceMsg(mes *tx.Message) error {
	return nil
}

func (pp *PushPool) GetPendingNonce() uint64 {
	return pp.pendingNonce
}

func (pp *PushPool) GetPendingMsg() []types.MsgID {
	pp.RLock()
	res := make([]types.MsgID, len(pp.pendingMsg))
	for mid := range pp.pendingMsg {
		res = append(res, mid)
	}
	pp.RUnlock()
	return res
}
