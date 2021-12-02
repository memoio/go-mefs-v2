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

	// todo: add txn store
	// need a different store
	ds store.KVStore

	activeRoles []uint64

	msgNum        uint16           // applied msg number of current height
	height        uint64           // next block height
	slot          uint64           // logical time
	chalEpoch     uint64           // next epoch
	chalEpochInfo *types.ChalEpoch // chal epoch
	root          types.MsgID      // for verify
	oInfo         map[orderKey]*orderInfo
	sInfo         map[uint64]*segPerUser // key: userID
	rInfo         map[uint64]*roleInfo

	validateChalEpoch     uint64
	validateChalEpochInfo *types.ChalEpoch
	validateRoot          types.MsgID
	validateOInfo         map[orderKey]*orderInfo
	validateSInfo         map[uint64]*segPerUser
	validateRInfo         map[uint64]*roleInfo

	hauf HandleAddUserFunc
}

func NewStateMgr(ds store.KVStore, ir api.IRole) *StateMgr {
	s := &StateMgr{
		IRole:                 ir,
		ds:                    ds,
		height:                0,
		activeRoles:           make([]uint64, 0, 16),
		root:                  beginRoot,
		validateRoot:          beginRoot,
		chalEpoch:             0,
		chalEpochInfo:         newChalEpoch(),
		validateChalEpochInfo: newChalEpoch(),
		oInfo:                 make(map[orderKey]*orderInfo),
		sInfo:                 make(map[uint64]*segPerUser),
		rInfo:                 make(map[uint64]*roleInfo),
	}

	s.load()

	return s
}

func (s *StateMgr) RegisterAddUserFunc(h HandleAddUserFunc) {
	s.Lock()
	s.hauf = h
	s.Unlock()
}

func (s *StateMgr) load() {
	// load keepers

	// load block height, epoch and uncompleted msgs
	key := store.NewKey(pb.MetaType_ST_BlockHeightKey)
	val, err := s.ds.Get(key)
	if err == nil && len(val) >= 18 {
		s.height = binary.BigEndian.Uint64(val[:8])
		s.slot = binary.BigEndian.Uint64(val[8:16])
		s.msgNum = binary.BigEndian.Uint16(val[16:])
	}

	// load root
	key = store.NewKey(pb.MetaType_ST_RootKey)
	val, err = s.ds.Get(key)
	if err == nil {
		rt, err := types.FromBytes(val)
		if err == nil {
			s.root = rt
			s.validateRoot = rt
		}
	}

	// load chal epoch
	key = store.NewKey(pb.MetaType_ST_ChalEpochKey)
	val, err = s.ds.Get(key)
	if err == nil && len(val) >= 8 {
		s.chalEpoch = binary.BigEndian.Uint64(val)
	}

	if s.chalEpoch == 0 {
		return
	}

	// load current chal
	key = store.NewKey(pb.MetaType_ST_ChalEpochKey, s.chalEpoch-1)
	val, err = s.ds.Get(key)
	if err == nil {
		s.chalEpochInfo.Deserialize(val)
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

func (s *StateMgr) loadNonce(roleID uint64) uint64 {
	key := store.NewKey(pb.MetaType_ST_RoleInfoKey, roleID)
	data, err := s.ds.Get(key)
	if err == nil && len(data) >= 8 {
		return binary.BigEndian.Uint64(data[:8])
	}
	return 0
}

func (s *StateMgr) saveNonce(roleID, nonce uint64) {
	key := store.NewKey(pb.MetaType_ST_RoleInfoKey, roleID)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, nonce)
	s.ds.Put(key, buf)
}

func (s *StateMgr) ApplyBlock(blk *tx.Block) (types.MsgID, error) {
	if blk == nil {
		// todo: commmit for apply all changes
		return s.root, nil
	}

	// todo: create new transcation

	if blk.Height != s.height {
		return s.root, xerrors.Errorf("apply block height is wrong: got %d, expected %d", blk.Height, s.height)
	}

	if blk.Slot <= s.slot {
		return s.root, xerrors.Errorf("apply block epoch is wrong: got %d, expected larger than %d", blk.Slot, s.slot)
	}

	b, err := blk.RawHeader.Serialize()
	if err != nil {
		return s.root, err
	}

	s.height++
	s.slot = blk.Slot
	s.msgNum = uint16(len(blk.Txs))

	key := store.NewKey(pb.MetaType_ST_BlockHeightKey)
	buf := make([]byte, 18)
	binary.BigEndian.PutUint64(buf[:8], s.height)
	binary.BigEndian.PutUint64(buf[8:16], s.slot)
	binary.BigEndian.PutUint16(buf[16:], s.msgNum)
	s.ds.Put(key, buf)

	key = store.NewKey(pb.MetaType_ST_BlockHeightKey, blk.Height)
	s.ds.Put(key, buf)

	s.newRoot(b)

	return s.root, nil
}

func (s *StateMgr) AppleyMsg(msg *tx.Message, tr *tx.Receipt) (types.MsgID, error) {
	if msg == nil {
		return s.root, nil
	}

	logger.Debug("block apply message:", msg.From, msg.Nonce, msg.Method, s.root)

	s.Lock()
	defer s.Unlock()

	ri, ok := s.rInfo[msg.From]
	if !ok {
		ri = &roleInfo{
			Nonce: s.loadNonce(msg.From),
		}
		s.rInfo[msg.From] = ri
	}

	if msg.Nonce != ri.Nonce {
		return s.root, xerrors.Errorf("wrong nonce for: %d, expeted %d, got %d", msg.From, ri.Nonce, msg.Nonce)
	}
	ri.Nonce++
	s.saveNonce(msg.From, ri.Nonce)
	s.newRoot(msg.Params)

	s.msgNum--
	key := store.NewKey(pb.MetaType_ST_BlockHeightKey, s.height-1)
	buf := make([]byte, 18)
	binary.BigEndian.PutUint64(buf[:8], s.height)
	binary.BigEndian.PutUint64(buf[8:16], s.slot)
	binary.BigEndian.PutUint16(buf[16:], s.msgNum)
	s.ds.Put(key, buf)

	// not apply wrong message; but update its nonce
	if tr.Err != 0 {
		logger.Debug("not apply wrong message")
		return s.root, nil
	}

	switch msg.Method {
	case tx.CreateFs:
		err := s.addUser(msg)
		if err != nil {
			return s.root, err
		}
	case tx.CreateBucket:
		err := s.addBucket(msg)
		if err != nil {
			return s.root, err
		}
	case tx.DataPreOrder:
		err := s.addOrder(msg)
		if err != nil {
			return s.root, err
		}
	case tx.DataOrder:
		err := s.addSeq(msg)
		if err != nil {
			return s.root, err
		}
	case tx.UpdateEpoch:
		err := s.updateChalEpoch(msg)
		if err != nil {
			return s.root, err
		}
	case tx.SegmentProof:
		err := s.addSegProof(msg)
		if err != nil {
			return s.root, err
		}
	default:
		return s.root, xerrors.Errorf("unsupported type: %d", msg.Method)
	}

	return s.root, nil
}
