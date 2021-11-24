package state

import (
	"sync"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/backend/wrap"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

type ValidateState struct {
	sync.RWMutex

	api.IRole

	ds store.KVStore

	height uint64

	oInfo map[orderKey]*orderInfo
	sInfo map[uint64]*segPerUser // key: userID
}

func NewValidateState(ds store.KVStore, ir api.IRole) *ValidateState {
	s := &ValidateState{
		IRole: ir,
		ds:    wrap.NewKVStore(StatePrefix, ds),
		oInfo: make(map[orderKey]*orderInfo),
		sInfo: make(map[uint64]*segPerUser),
	}

	return s
}

func (v *ValidateState) Reset() {
	v.oInfo = make(map[orderKey]*orderInfo)
	v.sInfo = make(map[uint64]*segPerUser)
}

func (v *ValidateState) ValidateMsg(msg *tx.Message) error {
	logger.Debug("validate message:", msg.From, msg.Nonce, msg.Method)
	switch msg.Method {
	case tx.CreateRole:
	case tx.CreateBucket:
	case tx.DataPreOrder:
	case tx.DataOrder:
	default:
		return ErrRes
	}

	return nil
}
