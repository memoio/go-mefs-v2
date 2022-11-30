package keeper

import (
	"context"

	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
)

func (k *KeeperNode) txMsgHandler(ctx context.Context, mes *tx.SignedMessage) error {
	logger.Debug("received pub msg: ", mes.From, mes.Nonce, mes.Method)
	return k.inp.SyncAddTxMessage(ctx, mes)
}

func (k *KeeperNode) putLfsMetaHandler(ctx context.Context, em *pb.EventMessage) error {
	sr := new(types.SignedRecord)

	err := sr.Deserialize(em.GetData())
	if err != nil {
		return err
	}

	return k.MetaStore().Put(sr.Key, em.GetData())
}
