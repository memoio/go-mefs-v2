package state

import (
	"encoding/binary"
	"sync"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/zeebo/blake3"
	"golang.org/x/xerrors"
)

// key: pb.MetaType_ST_RootKey; val: root []byte
type StateMgr struct {
	sync.RWMutex

	api.IRole

	// todo: txn store
	ds store.KVStore

	activeRoles []uint64

	height    uint64      // next block height
	epoch     uint64      // next epoch
	epochInfo *ChalEpoch  // chal epoch
	root      types.MsgID // for verify
	oInfo     map[orderKey]*orderInfo
	sInfo     map[uint64]*segPerUser // key: userID

	validateHeight    uint64
	validateEpoch     uint64
	validateEpochInfo *ChalEpoch
	validateRoot      types.MsgID
	validateOInfo     map[orderKey]*orderInfo
	validateSInfo     map[uint64]*segPerUser

	hasf HandleAddStripeFunc
}

func NewStateMgr(ds store.KVStore, ir api.IRole) *StateMgr {
	s := &StateMgr{
		IRole:             ir,
		ds:                ds,
		height:            0,
		activeRoles:       make([]uint64, 0, 16),
		root:              beginRoot,
		validateRoot:      beginRoot,
		epoch:             0,
		epochInfo:         newChalEpoch(),
		validateEpochInfo: newChalEpoch(),
		oInfo:             make(map[orderKey]*orderInfo),
		sInfo:             make(map[uint64]*segPerUser),
	}

	s.load()

	return s
}

func (s *StateMgr) RegisterAddStripeFunc(h HandleAddStripeFunc) {
	s.Lock()
	s.hasf = h
	s.Unlock()
}

func (s *StateMgr) load() {
	// load keepers

	// load block height
	key := store.NewKey(pb.MetaType_ST_BlockHeightKey)
	val, err := s.ds.Get(key)
	if err == nil && len(val) >= 8 {
		s.height = binary.BigEndian.Uint64(val)
	}

	// load root
	key = store.NewKey(pb.MetaType_ST_RootKey)
	val, err = s.ds.Get(key)
	if err == nil {
		rt, err := types.FromBytes(val)
		if err == nil {
			s.root = rt
		}
	}

	// load chal epoch
	key = store.NewKey(pb.MetaType_ST_EpochKey)
	val, err = s.ds.Get(key)
	if err == nil && len(val) >= 8 {
		s.epoch = binary.BigEndian.Uint64(val)
	}

	if s.epoch > 0 {
		// load chal seed
		key = store.NewKey(pb.MetaType_ST_EpochKey, s.epoch-1)
		val, err = s.ds.Get(key)
		if err == nil {
			s.epochInfo.Deserialize(val)
		}
	}
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

func (s *StateMgr) ApplyBlock(blk *tx.Block) (types.MsgID, error) {
	if blk.Height != s.height {
		return s.root, xerrors.Errorf("apply block is wrong: got %d, expected %d, %w", blk.Height, s.height, ErrBlockHeight)
	}

	b, err := blk.RawHeader.Serialize()
	if err != nil {
		return s.root, err
	}

	s.height++

	key := store.NewKey(pb.MetaType_ST_BlockHeightKey)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, s.height)
	s.ds.Put(key, buf)

	s.newRoot(b)

	return s.root, nil
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
	case tx.UpdateEpoch:
		return s.UpdateEpoch(msg)
	case tx.SegmentProof:
		return s.AddSegProof(msg)
	default:
		return s.root, ErrRes
	}
}
