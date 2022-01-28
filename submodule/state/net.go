package state

import (
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

func (s *StateMgr) updateNetAddr(msg *tx.Message, tds store.TxnStore) error {
	key := store.NewKey(pb.MetaType_ST_NetKey, msg.From)
	return tds.Put(key, msg.Params)
}

func (s *StateMgr) canUpdateNetAddr(msg *tx.Message) error {
	return nil
}
