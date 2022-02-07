package keeper

import (
	"context"
	"encoding/binary"

	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

func (k *KeeperNode) txMsgHandler(ctx context.Context, mes *tx.SignedMessage) error {
	logger.Debug("received pub msg: ", mes.From, mes.Nonce, mes.Method)
	return k.inp.AddTxMsg(ctx, mes)
}

func (k *KeeperNode) AddUsers(userID uint64) {
	key := store.NewKey(pb.MetaType_Chal_UsersKey)
	val, _ := k.MetaStore().Get(key)
	buf := make([]byte, len(val)+8)
	copy(buf[:len(val)], val)
	binary.BigEndian.PutUint64(buf[len(val):len(val)+8], userID)
	k.MetaStore().Put(key, buf)
}

func (k *KeeperNode) AddUP(userID, proID uint64) {
	key := store.NewKey(pb.MetaType_Chal_UsersKey, proID)
	val, _ := k.MetaStore().Get(key)
	buf := make([]byte, len(val)+8)
	copy(buf[:len(val)], val)
	binary.BigEndian.PutUint64(buf[len(val):len(val)+8], userID)
	k.MetaStore().Put(key, buf)

	key = store.NewKey(pb.MetaType_Chal_ProsKey, userID)
	val, _ = k.MetaStore().Get(key)
	buf = make([]byte, len(val)+8)
	copy(buf[:len(val)], val)
	binary.BigEndian.PutUint64(buf[len(val):len(val)+8], proID)
	k.MetaStore().Put(key, buf)
}
