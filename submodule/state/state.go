package state

import (
	"sync"

	"github.com/memoio/go-mefs-v2/api"
	pdpv2 "github.com/memoio/go-mefs-v2/lib/crypto/pdp/version2"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

// latest state for apply
type StateMgr struct {
	sync.RWMutex

	api.IRole

	ds store.KVStore

	activeRoles []uint64

	oInfo map[orderKey]*orderInfo
	sInfo map[uint64]*segPerUser // key: userID

	validateOInfo map[orderKey]*orderInfo
	validateSInfo map[uint64]*segPerUser
}

func NewStateMgr(ds store.KVStore, ir api.IRole) *StateMgr {
	s := &StateMgr{
		IRole:       ir,
		ds:          ds,
		activeRoles: make([]uint64, 0, 16),
		oInfo:       make(map[orderKey]*orderInfo),
		sInfo:       make(map[uint64]*segPerUser),
	}

	s.load()

	return s
}

func (s *StateMgr) load() {
	// load?
}

func (s *StateMgr) AppleyMsg(msg *tx.Message) error {
	logger.Debug("block apply message:", msg.From, msg.Nonce, msg.Method)
	switch msg.Method {
	case tx.CreateFs:
		pk := new(pdpv2.PublicKey)
		err := pk.Deserialize(msg.Params)
		if err != nil {
			return err
		}

		return s.AddUser(msg.From, pk)
	case tx.CreateBucket:
		tbp := new(tx.BucketParams)
		err := tbp.Deserialize(msg.Params)
		if err != nil {
			return err
		}
		return s.AddBucket(msg.From, tbp.BucketID, &tbp.BucketOption)
	case tx.DataPreOrder:
		so := new(types.SignedOrder)
		err := so.Deserialize(msg.Params)
		if err != nil {
			return err
		}
		return s.AddOrder(so)
	case tx.DataOrder:
		so := new(types.SignedOrderSeq)
		err := so.Deserialize(msg.Params)
		if err != nil {
			return err
		}
		return s.AddSeq(so)
	default:
		return ErrRes
	}
}
