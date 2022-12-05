package txPool

import (
	"context"
	"math"
	"math/big"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
)

type msgTo struct {
	ctime time.Time
	mtime time.Time
	msgID types.MsgID
	msg   *tx.SignedMessage
}

type pendingMsg struct {
	chainNonce uint64            // nonce on chain
	nonce      uint64            // pending nonce
	msgto      map[uint64]*msgTo // to push
}

var _ api.IChainPush = &PushPool{}

type PushPool struct {
	lk sync.RWMutex

	*SyncPool

	ctx context.Context

	localID uint64
	locals  []uint64
	pending map[uint64]*pendingMsg

	msgDone chan *blkDigest

	ready bool
}

func NewPushPool(ctx context.Context, sp *SyncPool) *PushPool {
	pp := &PushPool{
		SyncPool: sp,
		ctx:      ctx,

		locals:  make([]uint64, 0, 16),
		pending: make(map[uint64]*pendingMsg),

		msgDone: sp.msgDone,
		ready:   false,
	}

	pri, err := sp.RoleSelf(ctx)
	if err == nil {
		pp.localID = pri.RoleID
	}

	return pp
}

func (pp *PushPool) Start() {
	pp.SyncPool.start()
	logger.Debug("start push pool")
	go pp.syncPush()
}

func (pp *PushPool) syncPush() {
	tc := time.NewTicker(30 * time.Second)
	defer tc.Stop()

	for {
		time.Sleep(10 * time.Second)

		si, err := pp.SyncGetInfo(pp.ctx)
		if err != nil {
			continue
		}

		logger.Debug("wait sync; pool state: ", si.SyncedHeight, si.RemoteHeight, si.Status)
		if si.SyncedHeight == si.RemoteHeight && si.Status {
			logger.Info("sync complete; pool state: ", si.SyncedHeight, si.RemoteHeight, si.Status)
			break
		}
	}

	// need load pending msgs?

	pp.ready = true
	pp.inPush = true

	logger.Debug("push pool is ready")

	for {
		select {
		case <-pp.ctx.Done():
			return
		case bh := <-pp.msgDone:
			logger.Debug("process new block at: ", bh.height)

			pp.lk.Lock()
			for _, md := range bh.msgs {
				lpending, ok := pp.pending[md.From]
				if ok {
					logger.Debug("tx msg done: ", md.From, md.Nonce, md.ID)
					delete(lpending.msgto, md.Nonce)
					if lpending.chainNonce <= md.Nonce {
						lpending.chainNonce = md.Nonce + 1
					}
				}
			}
			pp.lk.Unlock()

		case <-tc.C:
			pp.lk.Lock()
			for _, lid := range pp.locals {
				lpending, ok := pp.pending[lid]
				if ok {
					res := make([]uint64, len(lpending.msgto))
					for nc := range lpending.msgto {
						res = append(res, nc)
					}

					for _, nc := range res {
						pmsg, ok := lpending.msgto[nc]
						if !ok {
							continue
						}

						if pmsg.msg.Nonce < lpending.chainNonce {
							// remove it
							delete(lpending.msgto, nc)
							continue
						}

						if time.Since(pmsg.ctime) > time.Hour {
							// remove it
							delete(lpending.msgto, nc)
							continue
						}

						// not re-pub mes
						if nc != pp.localID {
							pp.INetService.PublishTxMsg(pp.ctx, pmsg.msg)
							pmsg.mtime = time.Now()
							continue
						}

						if time.Since(pmsg.mtime) > 10*time.Minute {
							origID := pmsg.msgID

							gp := big.NewInt(10)
							if pmsg.msg.GasPrice != nil {
								gp = gp.Add(gp, pmsg.msg.GasPrice)
							}
							pmsg.msg.GasPrice = gp

							pmsg.msgID = pmsg.msg.Hash()
							sig, err := pp.RoleSign(pp.ctx, pmsg.msg.From, pmsg.msgID.Bytes(), types.SigSecp256k1)
							if err != nil {
								continue
							}
							pmsg.msg.Signature = sig

							// publish again
							pp.INetService.PublishTxMsg(pp.ctx, pmsg.msg)
							pmsg.mtime = time.Now()

							pp.PutTxMsgState(origID, &tx.MsgState{
								BlockID: pmsg.msgID,
								Height:  math.MaxUint64,
							})

							// need store?
						}
					}
				}
			}
			pp.lk.Unlock()
		}
	}
}

func (pp *PushPool) PushMessage(ctx context.Context, mes *tx.Message) (types.MsgID, error) {
	logger.Debug("add tx message to push pool: ", pp.ready, mes.From, mes.To, mes.Method)

	pp.lk.Lock()
	if !pp.ready {
		pp.lk.Unlock()
		return types.MsgID{}, xerrors.Errorf("push pool is not ready")
	}

	lp, ok := pp.pending[mes.From]
	if !ok {
		cNonce, err := pp.StateGetNonce(pp.ctx, mes.From)
		if err != nil {
			pp.lk.Unlock()
			return types.MsgID{}, err
		}
		lp = &pendingMsg{
			chainNonce: cNonce,
			nonce:      cNonce,
			msgto:      make(map[uint64]*msgTo),
		}
		pp.locals = append(pp.locals, mes.From)
		pp.pending[mes.From] = lp
	}

	// get nonce
	mes.Nonce = lp.nonce
	lp.nonce++
	pp.lk.Unlock()

	logger.Info("add tx message to push pool: ", pp.ready, mes.From, mes.To, mes.Nonce, mes.Method)

	mid := mes.Hash()
	// sign
	sig, err := pp.RoleSign(pp.ctx, mes.From, mid.Bytes(), types.SigSecp256k1)
	if err != nil {
		return mid, xerrors.Errorf("add tx message to push pool sign fail %s", err)
	}

	sm := &tx.SignedMessage{
		Message:   *mes,
		Signature: sig,
	}

	return pp.PushSignedMessage(ctx, sm)
}

func (pp *PushPool) PushSignedMessage(ctx context.Context, sm *tx.SignedMessage) (types.MsgID, error) {
	logger.Debug("add tx message signed to push pool: ", pp.ready, sm.From, sm.Nonce, sm.Method)

	mid := sm.Hash()

	// verify signature
	valid, err := pp.RoleVerify(pp.ctx, sm.From, mid.Bytes(), sm.Signature)
	if err != nil {
		return mid, xerrors.Errorf("add tx message to push pool verify fail %s", err)
	}

	if !valid {
		return mid, xerrors.Errorf("add tx message to push pool invalid sign")
	}

	pp.lk.Lock()
	lp, ok := pp.pending[sm.From]
	if !ok {
		cNonce, err := pp.StateGetNonce(pp.ctx, sm.From)
		if err != nil {
			pp.lk.Unlock()
			return mid, err
		}
		lp = &pendingMsg{
			chainNonce: cNonce,
			nonce:      cNonce,
			msgto:      make(map[uint64]*msgTo),
		}
		pp.pending[sm.From] = lp
	}

	// verify nonce
	if sm.Nonce < lp.chainNonce {
		return mid, xerrors.Errorf("%d nonce should be no less than chain nonde %d, got %d", sm.From, lp.chainNonce, sm.Nonce)
	}

	if sm.Nonce == lp.nonce {
		lp.nonce++
	} else if sm.Nonce > lp.nonce {
		return mid, xerrors.Errorf("%d nonce should be no larger than %d, got %d", sm.From, lp.nonce, sm.Nonce)
	}

	// replace it; if exist
	// TODO compare GasPrice?
	lp.msgto[sm.Nonce] = &msgTo{
		ctime: time.Now(),
		mtime: time.Now(),
		msg:   sm,
		msgID: mid,
	}
	pp.lk.Unlock()

	// store?
	if !ok {
		err := pp.PutTxMsg(sm, true)
		if err != nil {
			return mid, xerrors.Errorf("add tx message to push pool put fails %s", err)
		}
	}

	// to process pool
	if pp.inProcess {
		pp.msgChan <- sm
	}

	// push out immediately
	err = pp.INetService.PublishTxMsg(pp.ctx, sm)
	if err != nil {
		return mid, xerrors.Errorf("add tx message to push pool publish fails %s", err)
	}

	logger.Debug("add tx message signed to publish pool: ", pp.ready, sm.From, sm.Nonce, sm.Method)

	return mid, nil
}

func (pp *PushPool) ReplaceMsg(mes *tx.Message) error {
	return nil
}

func (pp *PushPool) PushGetPendingNonce(ctx context.Context, id uint64) (uint64, error) {
	pp.lk.Lock()
	defer pp.lk.Unlock()
	lp, ok := pp.pending[id]
	if ok {
		return lp.nonce, nil
	}

	cNonce, err := pp.StateGetNonce(pp.ctx, id)
	if err != nil {
		return 0, err
	}
	lp = &pendingMsg{
		chainNonce: cNonce,
		nonce:      cNonce,
		msgto:      make(map[uint64]*msgTo),
	}
	pp.locals = append(pp.locals, id)
	pp.pending[id] = lp

	return cNonce, nil
}

func (pp *PushPool) GetPendingMsg(ctx context.Context, id uint64) []types.MsgID {
	pp.lk.RLock()
	defer pp.lk.RUnlock()
	lp, ok := pp.pending[id]
	if ok {
		res := make([]types.MsgID, len(lp.msgto))
		for _, msg := range lp.msgto {
			res = append(res, msg.msgID)
		}
		return res
	}

	return nil
}
