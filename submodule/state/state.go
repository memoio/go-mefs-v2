package state

import (
	"context"
	"encoding/binary"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/zeebo/blake3"
	"go.opencensus.io/stats"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/submodule/metrics"
)

// key: pb.MetaType_ST_RootKey; val: root []byte
type StateMgr struct {
	lk sync.RWMutex

	api.IRole

	// todo: add txn store
	// need a different store
	ds store.KVStore

	genesisBlockID types.MsgID

	keepers   []uint64
	pros      []uint64
	users     []uint64
	threshold int
	blkID     types.MsgID

	version uint32
	height  uint64 // next block height
	slot    uint64 // logical time
	ceInfo  *chalEpochInfo
	root    types.MsgID // for verify
	oInfo   map[orderKey]*orderInfo
	sInfo   map[uint64]*segPerUser // key: userID
	rInfo   map[uint64]*roleInfo

	validateVersion uint32
	validateHeight  uint64 // next block height
	validateSlot    uint64 // logical time
	validateCeInfo  *chalEpochInfo
	validateRoot    types.MsgID
	validateOInfo   map[orderKey]*orderInfo
	validateSInfo   map[uint64]*segPerUser
	validateRInfo   map[uint64]*roleInfo

	handleAddRole     HanderAddRoleFunc
	handleAddUser     HandleAddUserFunc
	handleAddUP       HandleAddUPFunc
	handleAddPay      HandleAddPayFunc
	handleAddSeq      HandleAddSeqFunc
	handleDelSeg      HandleDelSegFunc
	handleCommitOrder HandleCommitOrderFunc
}

// base is role contract address
func NewStateMgr(base []byte, groupID uint64, thre int, ds store.KVStore, ir api.IRole) *StateMgr {
	buf := make([]byte, 8+len(base))
	copy(buf[:len(base)], base)
	binary.BigEndian.PutUint64(buf[len(base):len(base)+8], groupID)

	s := &StateMgr{
		IRole:          ir,
		ds:             ds,
		height:         0,
		threshold:      thre,
		keepers:        make([]uint64, 0, 16),
		pros:           make([]uint64, 0, 128),
		users:          make([]uint64, 0, 128),
		genesisBlockID: types.NewMsgID(buf),
		blkID:          types.NewMsgID(buf),

		version:      0,
		root:         types.NewMsgID(buf),
		validateRoot: types.NewMsgID(buf),
		ceInfo: &chalEpochInfo{
			epoch:    0,
			current:  newChalEpoch(types.NewMsgID(buf)),
			previous: newChalEpoch(types.NewMsgID(buf)),
		},
		oInfo: make(map[orderKey]*orderInfo),
		sInfo: make(map[uint64]*segPerUser),
		rInfo: make(map[uint64]*roleInfo),

		validateVersion: 0,
		validateCeInfo: &chalEpochInfo{
			epoch:    0,
			current:  newChalEpoch(types.NewMsgID(buf)),
			previous: newChalEpoch(types.NewMsgID(buf)),
		},
	}

	s.load()

	logger.Debug("start state mgr")

	return s
}

func (s *StateMgr) RegisterAddRoleFunc(h HanderAddRoleFunc) {
	s.lk.Lock()
	s.handleAddRole = h
	s.lk.Unlock()
}

func (s *StateMgr) RegisterAddUserFunc(h HandleAddUserFunc) {
	s.lk.Lock()
	s.handleAddUser = h
	s.lk.Unlock()
}

func (s *StateMgr) RegisterAddUPFunc(h HandleAddUPFunc) {
	s.lk.Lock()
	s.handleAddUP = h
	s.lk.Unlock()
}

func (s *StateMgr) RegisterAddSeqFunc(h HandleAddSeqFunc) {
	s.lk.Lock()
	s.handleAddSeq = h
	s.lk.Unlock()
}

func (s *StateMgr) RegisterDelSegFunc(h HandleDelSegFunc) {
	s.lk.Lock()
	s.handleDelSeg = h
	s.lk.Unlock()
}

func (s *StateMgr) RegisterAddPayFunc(h HandleAddPayFunc) {
	s.lk.Lock()
	s.handleAddPay = h
	s.lk.Unlock()
}

func (s *StateMgr) RegisterCommitOrderFunc(h HandleCommitOrderFunc) {
	s.lk.Lock()
	s.handleCommitOrder = h
	s.lk.Unlock()
}

func (s *StateMgr) API() *stateAPI {
	return &stateAPI{s}
}

func (s *StateMgr) load() {
	// load block height, epoch and uncompleted msgs
	key := store.NewKey(pb.MetaType_ST_BlockHeightKey)
	val, err := s.ds.Get(key)
	if err == nil && len(val) >= 16 {
		s.height = binary.BigEndian.Uint64(val[:8]) + 1
		s.slot = binary.BigEndian.Uint64(val[8:16])
	}

	for i, ue := range build.UpdateMap {
		if s.height >= ue && s.version < i {
			s.version = i
		}
	}

	s.validateVersion = s.version

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

	// load blk root
	if s.height > 0 {
		key = store.NewKey(pb.MetaType_ST_BlockHeightKey, s.height-1)
		val, err = s.ds.Get(key)
		if err == nil {
			rt, err := types.FromBytes(val)
			if err == nil {
				s.blkID = rt
			}
		}
	}

	logger.Debug("load state at: ", s.height, s.slot, s.root, s.blkID)

	// load keepers
	key = store.NewKey(pb.MetaType_ST_KeepersKey)
	val, err = s.ds.Get(key)
	if err == nil && len(val) >= 0 {
		for i := 0; i < len(val)/8; i++ {
			s.keepers = append(s.keepers, binary.BigEndian.Uint64(val[8*i:8*(i+1)]))
		}
	}

	// load pros
	key = store.NewKey(pb.MetaType_ST_ProsKey)
	val, err = s.ds.Get(key)
	if err == nil && len(val) >= 0 {
		for i := 0; i < len(val)/8; i++ {
			s.pros = append(s.pros, binary.BigEndian.Uint64(val[8*i:8*(i+1)]))
		}
	}

	// load users
	key = store.NewKey(pb.MetaType_ST_UsersKey)
	val, err = s.ds.Get(key)
	if err == nil && len(val) >= 0 {
		for i := 0; i < len(val)/8; i++ {
			s.users = append(s.users, binary.BigEndian.Uint64(val[8*i:8*(i+1)]))
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

func (s *StateMgr) newRoot(b []byte, tds store.TxnStore) error {
	h := blake3.New()
	h.Write(s.root.Bytes())
	h.Write(b)
	res := h.Sum(nil)
	s.root = types.NewMsgID(res)

	// store
	key := store.NewKey(pb.MetaType_ST_RootKey)
	err := tds.Put(key, s.root.Bytes())
	if err != nil {
		return err
	}

	return nil
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
	s.lk.Lock()
	defer s.lk.Unlock()

	if blk == nil {
		// todo: commmit for apply all changes
		return s.root, nil
	}

	nt := time.Now()

	if !blk.PrevID.Equal(s.blkID) {
		//logger.Errorf("apply wrong block at height %d, block prevID got: %s, expected: %s", blk.Height, blk.PrevID, s.blkID)
		return s.root, xerrors.Errorf("apply wrong block at height %d, state got: %s, expected: %s", blk.Height, s.root, blk.ParentRoot)
	}

	if !s.root.Equal(blk.ParentRoot) {
		//logger.Errorf("apply wrong block at height %d, state got: %s, expected: %s", blk.Height, blk.ParentRoot, s.root)
		return s.root, xerrors.Errorf("apply wrong block at height %d, blk root got: %s, expected: %s", blk.Height, blk.ParentRoot, s.root)
	}

	// it is necessary to new a txn in ApplyBlock every time, cuz some values in
	// old txn may be put but not committed, which should be dropped
	tds, err := s.ds.NewTxnStore(true)
	if err != nil {
		return s.root, xerrors.Errorf("create new txn fail: %w", err)
	}
	defer tds.Discard()

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
			return s.root, xerrors.Errorf("apply block not have enough signer, expected at least %d got %d", thr, blk.Len())
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
			return s.root, xerrors.Errorf("apply block not have enough valid signer, expected at least %d got %d", thr, len(sset))
		}
	} else {
		for _, msg := range blk.MsgSet.Msgs {
			if msg.Method != tx.AddRole {
				return s.root, xerrors.Errorf("apply block have invalid message at block zero")
			}
			pri := new(pb.RoleInfo)
			err := proto.Unmarshal(msg.Params, pri)
			if err != nil {
				return s.root, xerrors.Errorf("apply block have invalid message at block zero %w", err)
			}
			if pri.Type != pb.RoleInfo_Keeper {
				return s.root, xerrors.Errorf("apply block have invalid message at block zero")
			}
		}
	}

	b, err := blk.RawHeader.Serialize()
	if err != nil {
		return s.root, err
	}

	key := store.NewKey(pb.MetaType_ST_BlockHeightKey)
	buf := make([]byte, 16)
	binary.BigEndian.PutUint64(buf[:8], blk.Height)
	binary.BigEndian.PutUint64(buf[8:16], blk.Slot)
	err = tds.Put(key, buf)
	if err != nil {
		return s.root, err
	}
	blkID := blk.Hash()
	key = store.NewKey(pb.MetaType_ST_BlockHeightKey, blk.Height)
	err = tds.Put(key, blkID.Bytes())
	if err != nil {
		return s.root, err
	}

	err = s.newRoot(b, tds)
	if err != nil {
		return s.root, err
	}

	for i, msg := range blk.Msgs {
		// apply message
		msgDone := metrics.Timer(context.TODO(), metrics.TxMessageApply)
		_, err = s.applyMsg(&msg.Message, &blk.Receipts[i], tds)
		if err != nil {
			//logger.Error("apply message fail: ", msg.From, msg.Nonce, msg.Method, err)
			return s.root, xerrors.Errorf("apply msg %d %d %d at height %d fail %s", msg.From, msg.Nonce, msg.Method, blk.Height, err)
		}
		msgDone()
	}

	if !s.root.Equal(blk.Root) {
		//logger.Error("apply block has wrong state end at height %d, state got: %s, expected: %s", blk.Height, blk.Root, s.root)
		return s.root, xerrors.Errorf("apply has wrong state end at height %d, state got: %s, expected: %s", blk.Height, blk.Root, s.root)
	}

	// apply block ok, commit all changes
	err = tds.Commit()
	if err != nil {
		return s.root, xerrors.Errorf("apply block txn commit fail %w", err)
	}

	s.height++
	s.slot = blk.Slot
	s.blkID = blkID

	// after apply all msg, update version
	nextVer := s.version + 1
	ue, ok := build.UpdateMap[nextVer]
	if ok {
		if s.height >= ue {
			s.version = nextVer
		}
	}

	logger.Debug("block apply at: ", blk.Height, time.Since(nt))

	return s.root, nil
}

func (s *StateMgr) applyMsg(msg *tx.Message, tr *tx.Receipt, tds store.TxnStore) (types.MsgID, error) {
	if msg == nil {
		return s.root, nil
	}

	nt := time.Now()
	//logger.Debug("block apply message:", msg.From, msg.Method, msg.Nonce, s.root)

	ri, ok := s.rInfo[msg.From]
	if !ok {
		ri = s.loadRole(msg.From)
		s.rInfo[msg.From] = ri
	}

	if msg.Nonce != ri.val.Nonce {
		return s.root, xerrors.Errorf("wrong nonce for: %d, expeted %d, got %d", msg.From, ri.val.Nonce, msg.Nonce)
	}
	ri.val.Nonce++
	err := s.saveVal(msg.From, ri.val, tds)
	if err != nil {
		return s.root, err
	}

	err = s.newRoot(msg.Params, tds)
	if err != nil {
		return s.root, err
	}

	// not apply wrong message; but update its nonce
	if tr.Err != 0 {
		stats.Record(context.TODO(), metrics.TxMessageApplyFailure.M(1))
		//logger.Debug("not apply wrong message")
		return s.root, nil
	} else {
		stats.Record(context.TODO(), metrics.TxMessageApplySuccess.M(1))
	}

	switch msg.Method {
	case tx.AddRole:
		err := s.addRole(msg, tds)
		if err != nil {
			return s.root, err
		}
	case tx.CreateFs:
		err := s.addUser(msg, tds)
		if err != nil {
			return s.root, err
		}
	case tx.CreateBucket:
		err := s.addBucket(msg, tds)
		if err != nil {
			return s.root, err
		}
	case tx.PreDataOrder:
		err := s.createOrder(msg, tds)
		if err != nil {
			return s.root, err
		}
	case tx.AddDataOrder:
		err := s.addSeq(msg, tds)
		if err != nil {
			return s.root, err
		}
	case tx.CommitDataOrder:
		err := s.commitOrder(msg, tds)
		if err != nil {
			return s.root, err
		}
	case tx.SubDataOrder:
		err := s.subOrder(msg, tds)
		if err != nil {
			return s.root, err
		}
	case tx.SegmentFault:
		err := s.removeSeg(msg, tds)
		if err != nil {
			return s.root, err
		}
	case tx.UpdateChalEpoch:
		err := s.updateChalEpoch(msg, tds)
		if err != nil {
			return s.root, err
		}
	case tx.SegmentProof:
		err := s.addSegProof(msg, tds)
		if err != nil {
			return s.root, err
		}
	case tx.ConfirmPostIncome:
		err := s.addPay(msg, tds)
		if err != nil {
			return s.root, err
		}
	case tx.UpdateNet:
		err := s.updateNetAddr(msg, tds)
		if err != nil {
			return s.root, err
		}
	default:
		return s.root, xerrors.Errorf("unsupported method: %d", msg.Method)
	}

	logger.Debug("block apply message: ", msg.From, msg.Method, msg.Nonce, s.root, time.Since(nt))

	return s.root, nil
}
