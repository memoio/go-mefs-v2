package txPool

import (
	"context"
	"sync"
	"time"

	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

type msgTo struct {
	mtime time.Time
	msg   *tx.SignedMessage
}

type PushPool struct {
	sync.RWMutex

	*SyncPool

	ctx context.Context

	pendingNonce uint64                 // pending
	pendingMsg   map[types.MsgID]*msgTo // to push

	msgDone chan types.MsgID

	ready bool
}

func NewPushPool(ctx context.Context, sp *SyncPool) *PushPool {
	pp := &PushPool{
		SyncPool: sp,
		ctx:      ctx,

		pendingNonce: sp.GetNonce(sp.localID),
		pendingMsg:   make(map[types.MsgID]*msgTo),

		msgDone: sp.msgDone,
		ready:   false,
	}

	go pp.sync()

	return pp
}

func (pp *PushPool) sync() {
	tc := time.NewTicker(5 * time.Second)
	defer tc.Stop()

	for {
		ok := pp.GetSyncStatus()
		if ok {
			break
		}

		sh, rh := pp.GetSyncHeight()
		logger.Debug("sync pool state: ", sh, rh, pp.SyncPool.ready)
		time.Sleep(5 * time.Second)
	}

	pp.ready = true
	pp.pendingNonce = pp.GetNonce(pp.localID)
	logger.Debug("pool is ready")

	pp.inPush = true

	for {
		select {
		case <-pp.ctx.Done():
			return
		case mid := <-pp.msgDone:
			pp.Lock()
			logger.Debug("tx message done: ", mid.String())
			delete(pp.pendingMsg, mid)
			pp.Unlock()
		case <-tc.C:
			pp.RLock()
			for _, pmsg := range pp.pendingMsg {
				if time.Since(pmsg.mtime) > 5*time.Minute {
					pp.PushSignedMessage(pmsg.msg)
					pmsg.mtime = time.Now()
				}
			}
			pp.RUnlock()
		}
	}
}

func (pp *PushPool) PushMessage(mes *tx.Message) (types.MsgID, error) {
	logger.Debug("add tx message to push pool: ", pp.ready)

	pp.Lock()
	if !pp.ready {
		pp.Unlock()
		return types.MsgID{}, ErrNotReady
	}

	// get nonce
	mes.Nonce = pp.pendingNonce
	pp.pendingNonce++

	mid, err := mes.Hash()
	if err != nil {
		pp.Unlock()
		return mid, err
	}

	// sign
	sig, err := pp.RoleSign(pp.ctx, mid.Bytes(), types.SigSecp256k1)
	if err != nil {
		pp.Unlock()
		return mid, err
	}

	sm := &tx.SignedMessage{
		Message:   *mes,
		Signature: sig,
	}

	pp.pendingMsg[mid] = &msgTo{
		mtime: time.Now(),
		msg:   sm,
	}
	pp.Unlock()

	return pp.PushSignedMessage(sm)
}

func (pp *PushPool) PushSignedMessage(sm *tx.SignedMessage) (types.MsgID, error) {
	mid, err := sm.Hash()
	if err != nil {
		return mid, err
	}

	err = pp.PutTxMsg(sm)
	if err != nil {
		return mid, err
	}

	key := store.NewKey(pb.MetaType_TX_MessageKey, sm.From, sm.Nonce)
	sbyte, err := sm.Serialize()
	if err != nil {
		return mid, err
	}
	pp.ds.Put(key, sbyte)

	pp.Lock()
	pp.pendingMsg[mid] = &msgTo{
		mtime: time.Now(),
		msg:   sm,
	}
	pp.Unlock()

	logger.Debug("tx message: ", mid.String())

	// push out immediately
	err = pp.INetService.PublishTxMsg(pp.ctx, sm)
	if err != nil {
		return mid, err
	}

	return mid, nil
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
