package keeper

import (
	"context"

	"github.com/memoio/go-mefs-v2/lib/tx"
)

func (k *KeeperNode) txMsgHandler(ctx context.Context, mes *tx.SignedMessage) error {
	logger.Debug("received pub msg:", mes.Method, mes.From)
	return k.inp.AddTxMsg(ctx, mes)
}
