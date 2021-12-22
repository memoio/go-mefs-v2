package state

import (
	"encoding/binary"
	"sync"

	"github.com/gogo/protobuf/proto"
	"github.com/zeebo/blake3"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

// key: pb.MetaType_ST_RootKey; val: root []byte
type StateMgr struct {
	sync.RWMutex

	api.IRole

	// todo: add txn store
	// need a different store
	ds store.KVStore

	msgNum uint16 // applied msg number of current height
	height uint64 // next block height
	slot   uint64 // logical time

	keepers   []uint64
	threshold int
	ceInfo    *chalEpochInfo
	root      types.MsgID // for verify
	oInfo     map[orderKey]*orderInfo
	sInfo     map[uint64]*segPerUser // key: userID
	rInfo     map[uint64]*roleInfo

	validateHeight uint64 // next block height
	validateSlot   uint64 // logical time
	validateCeInfo *chalEpochInfo
	validateRoot   types.MsgID
	validateOInfo  map[orderKey]*orderInfo
	validateSInfo  map[uint64]*segPerUser
	validateRInfo  map[uint64]*roleInfo

	handleAddRole HanderAddRoleFunc
	handleAddUser HandleAddUserFunc
	handleAddUP   HandleAddUPFunc
	handleAddPay  HandleAddPayFunc
	handleAddSeq  HandleAddSeqFunc
	handleDelSeg  HandleDelSegFunc
}

func NewStateMgr(groupID uint64, thre int, ds store.KVStore, ir api.IRole) *StateMgr {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, groupID)

	s := &StateMgr{
		IRole:        ir,
		ds:           ds,
		height:       0,
		threshold:    thre,
		keepers:      make([]uint64, 0, 16),
		root:         types.NewMsgID(buf),
		validateRoot: types.NewMsgID(buf),
		ceInfo: &chalEpochInfo{
			epoch:    0,
			current:  newChalEpoch(groupID),
			previous: newChalEpoch(groupID),
		},
		oInfo: make(map[orderKey]*orderInfo),
		sInfo: make(map[uint64]*segPerUser),
		rInfo: make(map[uint64]*roleInfo),

		validateCeInfo: &chalEpochInfo{
			epoch:    0,
			current:  newChalEpoch(groupID),
			previous: newChalEpoch(groupID),
		},
	}

	s.load()

	return s
}

func (s *StateMgr) RegisterAddRoleFunc(h HanderAddRoleFunc) {
	s.Lock()
	s.handleAddRole = h
	s.Unlock()
}

func (s *StateMgr) RegisterAddUserFunc(h HandleAddUserFunc) {
	s.Lock()
	s.handleAddUser = h
	s.Unlock()
}

func (s *StateMgr) RegisterAddUPFunc(h HandleAddUPFunc) {
	s.Lock()
	s.handleAddUP = h
	s.Unlock()
}

func (s *StateMgr) RegisterAddSeqFunc(h HandleAddSeqFunc) {
	s.Lock()
	s.handleAddSeq = h
	s.Unlock()
}

func (s *StateMgr) RegisterDelSegFunc(h HandleDelSegFunc) {
	s.Lock()
	s.handleDelSeg = h
	s.Unlock()
}

func (s *StateMgr) RegisterAddPayFunc(h HandleAddPayFunc) {
	s.Lock()
	s.handleAddPay = h
	s.Unlock()
}

func (s *StateMgr) API() *stateAPI {
	return &stateAPI{s}
}

func (s *StateMgr) load() {
	// load keepers
	key := store.NewKey(pb.MetaType_ST_KeepersKey)
	val, err := s.ds.Get(key)
	if err == nil && len(val) >= 0 {
		for i := 0; i < len(val)/8; i++ {
			s.keepers = append(s.keepers, binary.BigEndian.Uint64(val[8*i:8*(i+1)]))
		}
	}

	// load block height, epoch and uncompleted msgs
	key = store.NewKey(pb.MetaType_ST_BlockHeightKey)
	val, err = s.ds.Get(key)
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
		s.ceInfo.epoch = binary.BigEndian.Uint64(val)
	}

	if s.ceInfo.epoch == 0 {
		return
	}

	// load current chal
	if s.ceInfo.epoch > 0 {
		key = store.NewKey(pb.MetaType_ST_ChalEpochKey, s.ceInfo.epoch-1)
		val, err = s.ds.Get(key)
		if err == nil {
			s.ceInfo.current.Deserialize(val)
		}
	}

	if s.ceInfo.epoch > 1 {
		key = store.NewKey(pb.MetaType_ST_ChalEpochKey, s.ceInfo.epoch-2)
		val, err = s.ds.Get(key)
		if err == nil {
			s.ceInfo.previous.Deserialize(val)
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

func (s *StateMgr) getThreshold() int {
	thres := 2 * (len(s.keepers) + 1) / 3
	if thres > s.threshold {
		return thres
	} else {
		return s.threshold
	}
}

// block 0 is special: only accept addKeeper; and msg len >= threshold
func (s *StateMgr) ApplyBlock(blk *tx.SignedBlock) (types.MsgID, error) {
	s.Lock()
	defer s.Unlock()

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

	if blk.Height > 0 {
		// verify sign
		thr := s.getThreshold()
		if blk.Len() < thr {
			return s.root, xerrors.Errorf("not have enough signer, expected at least %d got %d", thr, blk.Len())
		}

		sset := make(map[uint64]struct{}, blk.Len())
		for _, signer := range blk.Signer {
			for _, kid := range s.keepers {
				if signer == kid {
					sset[signer] = struct{}{}
					break
				}
			}
		}

		if len(sset) < thr {
			return s.root, xerrors.Errorf("not have enough valid signer, expected at least %d got %d", thr, len(sset))
		}
	} else {
		for _, msg := range blk.MsgSet.Msgs {
			if msg.Method != tx.AddRole {
				return s.root, xerrors.Errorf("have invalid message at block zero")
			}
			pri := new(pb.RoleInfo)
			err := proto.Unmarshal(msg.Params, pri)
			if err != nil {
				return s.root, xerrors.Errorf("have invalid message at block zero %w", err)
			}
			if pri.Type != pb.RoleInfo_Keeper {
				return s.root, xerrors.Errorf("have invalid message at block zero")
			}
		}
	}

	b, err := blk.RawHeader.Serialize()
	if err != nil {
		return s.root, err
	}

	s.height++
	s.slot = blk.Slot
	s.msgNum = uint16(len(blk.Msgs))

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
		ri = s.loadRole(msg.From)
		s.rInfo[msg.From] = ri
	}

	if msg.Nonce != ri.val.Nonce {
		return s.root, xerrors.Errorf("wrong nonce for: %d, expeted %d, got %d", msg.From, ri.val.Nonce, msg.Nonce)
	}
	ri.val.Nonce++
	s.saveVal(msg.From, ri.val)
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
	case tx.AddRole:
		err := s.addRole(msg)
		if err != nil {
			return s.root, err
		}
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
	case tx.SegmentFault:
		err := s.removeSeg(msg)
		if err != nil {
			return s.root, err
		}
	case tx.UpdateChalEpoch:
		err := s.updateChalEpoch(msg)
		if err != nil {
			return s.root, err
		}
	case tx.SegmentProof:
		err := s.addSegProof(msg)
		if err != nil {
			return s.root, err
		}
	case tx.PostIncome:
		err := s.addPay(msg)
		if err != nil {
			return s.root, err
		}
	default:
		return s.root, xerrors.Errorf("unsupported method: %d", msg.Method)
	}

	return s.root, nil
}
