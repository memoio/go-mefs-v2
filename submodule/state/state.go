package state

import (
	"sync"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/zeebo/blake3"
)

// key: pb.MetaType_ST_RootKey; val: root []byte
type StateMgr struct {
	sync.RWMutex

	api.IRole

	// todo: txn store
	ds store.KVStore

	activeRoles []uint64

	root  types.MsgID // for verify
	oInfo map[orderKey]*orderInfo
	sInfo map[uint64]*segPerUser // key: userID

	validateRoot  types.MsgID
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
	key := store.NewKey(pb.MetaType_ST_RootKey)
	val, err := s.ds.Get(key)
	if err == nil {
		rt, err := types.FromBytes(val)
		if err == nil {
			s.root = rt
			return
		}
	}
	s.root = beginRoot
}

func (s *StateMgr) newRoot(b []byte) {
	h := blake3.New()
	h.Write(s.root.Bytes())
	h.Write(b)
	res := h.Sum(nil)
	s.root = types.NewMsgID(res)

	// store
	key := store.NewKey(pb.MetaType_ST_RootKey)
	s.ds.Put(key, s.root.Bytes())
}

func (s *StateMgr) GetRoot() types.MsgID {
	return s.root
}

func (s *StateMgr) AppleyMsg(msg *tx.Message) (types.MsgID, error) {
	if msg == nil {
		return s.root, nil
	}

	logger.Debug("block apply message:", msg.From, msg.Nonce, msg.Method, s.root)
	switch msg.Method {
	case tx.CreateFs:
		return s.AddUser(msg)
	case tx.CreateBucket:
		return s.AddBucket(msg)
	case tx.DataPreOrder:
		return s.AddOrder(msg)
	case tx.DataOrder:
		return s.AddSeq(msg)
	default:
		return s.root, ErrRes
	}
}
