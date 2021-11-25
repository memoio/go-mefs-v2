package txPool

import (
	"context"
	"sync"
	"time"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

type msgTo struct {
	mtime time.Time
	msg   *tx.SignedMessage
}

type pendingMsg struct {
	nonce uint64                 // pending
	msg   map[types.MsgID]*msgTo // to push
}

var _ api.IChain = &pushAPI{}

type pushAPI struct {
	*PushPool
}

type PushPool struct {
	sync.RWMutex

	*SyncPool

	ctx context.Context

	pending map[uint64]*pendingMsg

	msgDone chan *tx.MessageDigest

	ready bool
}

func NewPushPool(ctx context.Context, sp *SyncPool) *PushPool {
	pp := &PushPool{
		SyncPool: sp,
		ctx:      ctx,

		pending: make(map[uint64]*pendingMsg),

		msgDone: sp.msgDone,
		ready:   false,
	}

	pp.pending[sp.localID] = &pendingMsg{
		nonce: sp.GetNonce(ctx, sp.localID),
		msg:   make(map[types.MsgID]*msgTo),
	}

	// load unfinished

	go pp.sync()

	return pp
}

func (pp *PushPool) Ready() bool {
	return pp.ready
}

func (pp *PushPool) sync() {
	tc := time.NewTicker(5 * time.Second)
	defer tc.Stop()

	for {
		ok := pp.GetSyncStatus(pp.ctx)
		if ok {
			break
		}

		sh, rh := pp.GetSyncHeight(pp.ctx)
		logger.Debug("wait sync; pool state: ", sh, rh, pp.SyncPool.ready)
		time.Sleep(5 * time.Second)
	}

	pp.ready = true
	lpending := pp.pending[pp.localID]
	lpending.nonce = pp.GetNonce(pp.ctx, pp.localID)
	logger.Debug("pool is ready")

	pp.inPush = true

	for {
		select {
		case <-pp.ctx.Done():
			return
		case md := <-pp.msgDone:
			pp.Lock()
			logger.Debug("tx message done: ", md.ID)
			lpending, ok := pp.pending[md.From]
			if ok {
				delete(lpending.msg, md.ID)
			}
			pp.Unlock()
		case <-tc.C:
			pp.RLock()
			lpending, ok := pp.pending[pp.localID]
			if ok {
				for _, pmsg := range lpending.msg {
					if time.Since(pmsg.mtime) > 5*time.Minute {
						pp.PushSignedMessage(pp.ctx, pmsg.msg)
						pmsg.mtime = time.Now()
					}
				}
			}

			pp.RUnlock()
		}
	}
}

func (pp *PushPool) PushMessage(ctx context.Context, mes *tx.Message) (types.MsgID, error) {
	logger.Debug("add tx message to push pool: ", pp.ready, mes.From, mes.Method)

	pp.Lock()
	if !pp.ready {
		pp.Unlock()
		return types.MsgID{}, ErrNotReady
	}

	lp, ok := pp.pending[mes.From]
	if !ok {
		lp = &pendingMsg{
			nonce: pp.GetNonce(ctx, mes.From),
			msg:   make(map[types.MsgID]*msgTo),
		}
		pp.pending[mes.From] = lp
	}

	// get nonce
	mes.Nonce = lp.nonce
	lp.nonce++

	mid, err := mes.Hash()
	if err != nil {
		pp.Unlock()
		logger.Warn("add tx message to push pool: ", err)
		return mid, err
	}

	// sign
	sig, err := pp.RoleSign(pp.ctx, pp.localID, mid.Bytes(), types.SigSecp256k1)
	if err != nil {
		pp.Unlock()
		logger.Warn("add tx message to push pool: ", err)
		return mid, err
	}

	sm := &tx.SignedMessage{
		Message:   *mes,
		Signature: sig,
	}
	pp.Unlock()

	return pp.PushSignedMessage(ctx, sm)
}

func (pp *PushPool) PushSignedMessage(ctx context.Context, sm *tx.SignedMessage) (types.MsgID, error) {
	logger.Debug("add tx signed message to push pool: ", pp.ready)
	mid, err := sm.Hash()
	if err != nil {
		logger.Warn("add tx signed message to push pool: ", err)
		return mid, err
	}

	err = pp.PutTxMsg(sm)
	if err != nil {
		logger.Warn("add tx signed message to push pool: ", err)
		return mid, err
	}

	key := store.NewKey(pb.MetaType_TX_MessageKey, sm.From, sm.Nonce)
	pp.ds.Put(key, mid.Bytes())

	pp.Lock()
	lp, ok := pp.pending[sm.From]
	if !ok {
		lp = &pendingMsg{
			nonce: pp.GetNonce(ctx, sm.From),
			msg:   make(map[types.MsgID]*msgTo),
		}
		pp.pending[sm.From] = lp
	}

	lp.msg[mid] = &msgTo{
		mtime: time.Now(),
		msg:   sm,
	}
	pp.Unlock()

	logger.Debug("tx message: ", mid.String())

	// push out immediately
	err = pp.INetService.PublishTxMsg(pp.ctx, sm)
	if err != nil {
		logger.Warn("add tx signed message to push pool: ", err)
		return mid, err
	}

	return mid, nil
}

func (pp *PushPool) ReplaceMsg(mes *tx.Message) error {
	return nil
}

func (pp *PushPool) GetPendingNonce(ctx context.Context, id uint64) uint64 {
	pp.RLock()
	defer pp.RUnlock()
	lp, ok := pp.pending[id]
	if ok {
		return lp.nonce
	}
	return 0
}

func (pp *PushPool) GetPendingMsg(ctx context.Context, id uint64) []types.MsgID {
	pp.RLock()
	defer pp.RUnlock()
	lp, ok := pp.pending[id]
	if ok {
		res := make([]types.MsgID, len(lp.msg))
		for mid := range lp.msg {
			res = append(res, mid)
		}
		return res
	}

	return nil
}
