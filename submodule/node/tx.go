package node

import (
	"context"

	"go.opencensus.io/stats"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/submodule/metrics"
)

func (n *BaseNode) PushMessage(ctx context.Context, mes *tx.Message) (types.MsgID, error) {
	stats.Record(context.TODO(), metrics.TxMessagePush.M(1))

	if !n.isProxy {
		logger.Info("push message local")
		return n.LPP.PushMessage(ctx, mes)
	}

	n.lk.Lock()
	defer n.lk.Unlock()

	nonce, err := n.rcp.PushGetPendingNonce(ctx, mes.From)
	if err != nil {
		return types.MsgID{}, err
	}
	mes.Nonce = nonce

	mid := mes.Hash()
	// sign
	sig, err := n.RoleSign(ctx, mes.From, mid.Bytes(), types.SigSecp256k1)
	if err != nil {
		return mid, xerrors.Errorf("add tx message to push pool sign fail %s", err)
	}

	sm := &tx.SignedMessage{
		Message:   *mes,
		Signature: sig,
	}

	logger.Debug("push message remote: ", mes.From, mes.To, mes.Nonce, mes.Method)

	nmid, err := n.rcp.PushSignedMessage(ctx, sm)
	if err != nil {
		logger.Warn("push message remote: ", mes.From, mes.To, mes.Nonce, mes.Method, err)
		return mid, err
	}

	return nmid, nil
}

func (n *BaseNode) PushGetPendingNonce(ctx context.Context, id uint64) (uint64, error) {
	if n.isProxy {
		return n.rcp.PushGetPendingNonce(ctx, id)
	}

	return n.LPP.PushGetPendingNonce(ctx, id)
}

func (n *BaseNode) PushSignedMessage(ctx context.Context, sm *tx.SignedMessage) (types.MsgID, error) {
	if n.isProxy {
		return n.rcp.PushSignedMessage(ctx, sm)
	}

	return n.LPP.PushSignedMessage(ctx, sm)
}
