package state

import (
	"sync"

	"github.com/fxamacker/cbor/v2"
	"github.com/gogo/protobuf/proto"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/backend/wrap"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

// latest state for apply
type StateMgr struct {
	sync.RWMutex

	api.IRole

	ds store.KVStore

	height uint64 // block height applied

	keepers []uint64
	users   []uint64
	pros    []uint64

	oInfo map[orderKey]*orderInfo
	sInfo map[uint64]*segPerUser // key: userID
}

func NewStateDB(ds store.KVStore, ir api.IRole) *StateMgr {
	s := &StateMgr{
		IRole: ir,
		ds:    wrap.NewKVStore(StatePrefix, ds),
		users: make([]uint64, 0, 16),
		pros:  make([]uint64, 0, 16),
		oInfo: make(map[orderKey]*orderInfo),
		sInfo: make(map[uint64]*segPerUser),
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
	case tx.CreateRole:
		pri := new(pb.RoleInfo)
		err := proto.Unmarshal(msg.Params, pri)
		if err != nil {
			return err
		}
		return s.AddRole(pri)
	case tx.CreateBucket:
		pro := new(tx.BucketParams)
		err := cbor.Unmarshal(msg.Params, pro)
		if err != nil {
			return err
		}
		return s.AddBucket(msg.From, pro.BucketID, &pro.BucketOption)
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
