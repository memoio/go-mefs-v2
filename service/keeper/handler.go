package keeper

import (
	"context"

	"github.com/memoio/go-mefs-v2/lib/tx"
)

func (k *KeeperNode) txMsgHandler(ctx context.Context, mes *tx.SignedMessage) error {
	logger.Debug("received pub msg:", mes.From, mes.Nonce, mes.Method)
	return k.inp.AddTxMsg(ctx, mes)
}
