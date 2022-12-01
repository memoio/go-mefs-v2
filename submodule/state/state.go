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
	"github.com/memoio/go-mefs-v2/lib/backend/smt"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/submodule/metrics"
)

// not in smt
// store.NewKey(pb.MetaType_ST_BlockHeightKey), val: height
// store.NewKey(pb.MetaType_ST_BlockHeightKey, height), val: // blk hash(20), slot(8)
// store.NewKey(pb.MetaType_ST_RootKey), val: state hash
// store.NewKey(pb.MetaType_ST_RootKey, height), val: state hash

// store.NewKey(pb.MetaType_ST_KeepersKey)
// store.NewKey(pb.MetaType_ST_ProsKey)
// store.NewKey(pb.MetaType_ST_UsersKey)

type StateMgr struct {
	lk sync.RWMutex

	api.IRole

	ds  store.KVStore
	smt store.SMTStore
	tds store.TxnStore

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
}

// base is role contract address
func NewStateMgr(base []byte, groupID uint64, thre int, ds store.KVStore, ir api.IRole) *StateMgr {
	buf := make([]byte, 8+len(base))
	copy(buf[:len(base)], base)
	binary.BigEndian.PutUint64(buf[len(base):len(base)+8], groupID)

	s := &StateMgr{
		IRole:          ir,
		ds:             ds,
		smt:            smt.NewSMTree(nil, ds, ds),
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

func (s *StateMgr) API() *stateAPI {
	return &stateAPI{s}
}

func (s *StateMgr) load() {
	// load block height, epoch
	key := store.NewKey(pb.MetaType_ST_BlockHeightKey)
	val, err := s.ds.Get(key)
	if err == nil {
		if len(val) >= 8 {
			s.height = binary.BigEndian.Uint64(val[:8]) + 1
		}
		if len(val) >= 16 {
			s.slot = binary.BigEndian.Uint64(val[8:16])
		}
	}

	for i, ue := range build.UpdateMap {
		if s.height >= ue && s.version < i {
			s.version = i
		}
	}
	s.validateVersion = s.version

	// load slot and blk root at some height
	if s.height > 0 {
		key = store.NewKey(pb.MetaType_ST_BlockHeightKey, s.height-1)
		val, err = s.ds.Get(key)
		if err == nil {
			if len(val) >= types.MsgLen {
				rt, err := types.FromBytes(val[:types.MsgLen])
				if err == nil {
					s.blkID = rt
				}
			}

			if len(val) >= types.MsgLen+8 {
				s.slot = binary.BigEndian.Uint64(val[types.MsgLen : types.MsgLen+8])
			}
		}
	}

	// load state
	key = store.NewKey(pb.MetaType_ST_RootKey)
	val, err = s.ds.Get(key)
	if err == nil {
		switch len(val) {
		case 32:
			s.root = types.NewMsgID(val)
			// smt set root
			s.smt.SetRoot(val)
		default:
			// 22
			rt, err := types.FromBytes(val)
			if err == nil {
				s.root = rt
			}
		}
	}

	// load state at some height
	if s.height > 0 {
		key = store.NewKey(pb.MetaType_ST_RootKey, s.height-1)
		val, err = s.ds.Get(key)
		if err == nil {
			switch len(val) {
			case 32:
				s.root = types.NewMsgID(val)
				// smt set root
				s.smt.SetRoot(val)
			default:
				rt, err := types.FromBytes(val)
				if err == nil {
					s.root = rt
				}
			}
		}
	}

	s.validateRoot = s.root

	logger.Debug("load state at: ", s.height, s.slot, s.root, s.blkID)

	// load keepers
	key = store.NewKey(pb.MetaType_ST_KeepersKey)
	val, err = s.ds.Get(key)
	if err == nil && len(val) >= 0 {
		for i := 0; i < len(val)/8; i++ {
			kid := binary.BigEndian.Uint64(val[8*i : 8*(i+1)])
			key := store.NewKey(pb.MetaType_ST_RoleBaseKey, kid)
			ok, err := s.has(key)
			if err == nil && ok {
				s.keepers = append(s.keepers, kid)
			}
		}
	}

	// load pros
	key = store.NewKey(pb.MetaType_ST_ProsKey)
	val, err = s.ds.Get(key)
	if err == nil && len(val) >= 0 {
		for i := 0; i < len(val)/8; i++ {
			kid := binary.BigEndian.Uint64(val[8*i : 8*(i+1)])
			key := store.NewKey(pb.MetaType_ST_RoleBaseKey, kid)
			ok, err := s.has(key)
			if err == nil && ok {
				s.pros = append(s.pros, kid)
			}
		}
	}

	// load users
	key = store.NewKey(pb.MetaType_ST_UsersKey)
	val, err = s.ds.Get(key)
	if err == nil && len(val) >= 0 {
		for i := 0; i < len(val)/8; i++ {
			kid := binary.BigEndian.Uint64(val[8*i : 8*(i+1)])
			key := store.NewKey(pb.MetaType_ST_RoleBaseKey, kid)
			ok, err := s.has(key)
			if err == nil && ok {
				s.users = append(s.users, kid)
			}
		}
	}

	// load chal epoch
	key = store.NewKey(pb.MetaType_ST_ChalEpochKey)
	val, err = s.get(key)
	if err == nil && len(val) >= 8 {
		s.ceInfo.epoch = binary.BigEndian.Uint64(val)
	}

	if s.ceInfo.epoch == 0 {
		return
	}

	// load current chal
	if s.ceInfo.epoch > 0 {
		key = store.NewKey(pb.MetaType_ST_ChalEpochKey, s.ceInfo.epoch-1)
		val, err = s.get(key)
		if err == nil {
			s.ceInfo.current.Deserialize(val)
		}
	}

	if s.ceInfo.epoch > 1 {
		key = store.NewKey(pb.MetaType_ST_ChalEpochKey, s.ceInfo.epoch-2)
		val, err = s.get(key)
		if err == nil {
			s.ceInfo.previous.Deserialize(val)
		}
	}
}

func (s *StateMgr) newRoot(b []byte) error {
	key := store.NewKey(pb.MetaType_ST_RootKey)

	if s.version < build.SMTVersion {
		h := blake3.New()
		h.Write(s.root.Bytes())
		h.Write(b)
		res := h.Sum(nil)
		s.root = types.NewMsgID(res)

		err := s.tds.Put(key, s.root.Bytes())
		if err != nil {
			return err
		}
	} else {
		s.put(key, b)
	}

	return nil
}

func (s *StateMgr) saveRoot() error {
	if s.version >= build.SMTVersion {
		s.root = types.NewMsgID(s.smt.Root())
		key := store.NewKey(pb.MetaType_ST_RootKey)
		err := s.tds.Put(key, s.smt.Root())
		if err != nil {
			return err
		}

		key = store.NewKey(pb.MetaType_ST_RootKey, s.height)
		err = s.tds.Put(key, s.smt.Root())
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *StateMgr) put(key, value []byte) error {
	if s.version < build.SMTVersion {
		return s.tds.Put(key, value)
	} else {
		return s.smt.Put(key, value)
	}
}

func (s *StateMgr) get(key []byte) ([]byte, error) {
	val, err := s.smt.Get(key)
	if err == nil {
		return val, nil
	}

	if s.tds != nil {
		return s.tds.Get(key)
	} else {
		return s.ds.Get(key)
	}
}

func (s *StateMgr) has(key []byte) (bool, error) {
	has, err := s.smt.Has(key)
	if err == nil || has {
		return has, nil
	}

	if s.tds != nil {
		return s.tds.Has(key)
	} else {
		return s.ds.Has(key)
	}
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
		return s.root, nil
	}

	nt := time.Now()

	if blk.Height != s.height {
		return s.root, xerrors.Errorf("apply block height is wrong: got %d, expected %d", blk.Height, s.height)
	}

	if blk.Slot <= s.slot {
		return s.root, xerrors.Errorf("apply block epoch is wrong: got %d, expected larger than %d", blk.Slot, s.slot)
	}

	if !blk.PrevID.Equal(s.blkID) {
		//logger.Errorf("apply wrong block at height %d, block prevID got: %s, expected: %s", blk.Height, blk.PrevID, s.blkID)
		// set height to previous one for smt recovery
		if blk.Height > 0 && s.version >= build.SMTVersion {
			key := store.NewKey(pb.MetaType_ST_BlockHeightKey)
			buf := make([]byte, 8)
			newH := blk.Height - 1
			if blk.Height > 1 {
				newH = blk.Height - 2
			}
			binary.BigEndian.PutUint64(buf[:8], newH)
			err := s.ds.Put(key, buf)
			if err != nil {
				return s.root, err
			}
		}

		return s.root, xerrors.Errorf("apply wrong block at height %d, block got: %s, expected: %s", blk.Height, blk.PrevID, s.blkID)
	}

	if !s.root.Equal(blk.ParentRoot) {
		//logger.Errorf("apply wrong block at height %d, state got: %s, expected: %s", blk.Height, blk.ParentRoot, s.root)

		// set height to previous one for smt recovery
		if blk.Height > 0 && s.version >= build.SMTVersion {
			key := store.NewKey(pb.MetaType_ST_BlockHeightKey)
			buf := make([]byte, 8)
			newH := blk.Height - 1
			if blk.Height > 1 {
				newH = blk.Height - 2
			}
			binary.BigEndian.PutUint64(buf[:8], newH)
			err := s.ds.Put(key, buf)
			if err != nil {
				return s.root, err
			}
		}

		return s.root, xerrors.Errorf("apply wrong block at height %d, state got: %s, expected: %s", blk.Height, blk.ParentRoot, s.root)
	}

	// it is necessary to new a txn in ApplyBlock every time, cuz some values in
	// old txn may be put but not committed, which should be dropped
	var err error
	s.tds, err = s.ds.NewTxnStore(true)
	if err != nil {
		return s.root, xerrors.Errorf("create new txn fail: %w", err)
	}
	defer func() {
		s.tds.Discard()
		s.tds = nil
	}()

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
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf[:8], blk.Height)
	err = s.tds.Put(key, buf)
	if err != nil {
		return s.root, err
	}

	blkID := blk.Hash()
	buf = make([]byte, types.MsgLen+8)
	copy(buf[:types.MsgLen], blkID.Bytes())
	binary.BigEndian.PutUint64(buf[types.MsgLen:types.MsgLen+8], blk.Slot)

	// slot, blk hash
	key = store.NewKey(pb.MetaType_ST_BlockHeightKey, blk.Height)
	err = s.tds.Put(key, buf)
	if err != nil {
		return s.root, err
	}

	// change state root
	err = s.newRoot(b)
	if err != nil {
		return s.root, err
	}

	for i, msg := range blk.Msgs {
		// apply message
		msgDone := metrics.Timer(context.TODO(), metrics.TxMessageApply)
		_, err = s.applyMsg(&msg.Message, &blk.Receipts[i])
		if err != nil {
			//logger.Error("apply message fail: ", msg.From, msg.Nonce, msg.Method, err)
			return s.root, xerrors.Errorf("apply msg %d %d %d at height %d fail %s", msg.From, msg.Nonce, msg.Method, blk.Height, err)
		}
		msgDone()
	}

	if s.version < build.SMTVersion && !s.root.Equal(blk.Root) {
		logger.Errorf("apply block has wrong state end at height %d, state got: %s, expected: %s", blk.Height, blk.Root, s.root)
		return s.root, xerrors.Errorf("apply has wrong state end at height %d, state got: %s, expected: %s", blk.Height, blk.Root, s.root)
	}

	// apply block ok, commit smt and update new root
	s.smt.SetRoot(s.smt.Root())

	err = s.saveRoot()
	if err != nil {
		return s.root, xerrors.Errorf("save smt root fail %w", err)
	}

	// apply block and save root ok, commit s.tds
	err = s.tds.Commit()
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

// TODO: modify s.tds.put() to smt.put()
// and s.tds.get() to smt.get()
func (s *StateMgr) applyMsg(msg *tx.Message, tr *tx.Receipt) (types.MsgID, error) {
	if msg == nil {
		return s.root, nil
	}

	nt := time.Now()
	//logger.Debug("block apply message: ", msg.From, msg.Method, msg.Nonce, s.root)

	ri, ok := s.rInfo[msg.From]
	if !ok {
		ri = s.loadRole(msg.From)
		s.rInfo[msg.From] = ri
	}

	if msg.Nonce != ri.val.Nonce {
		return s.root, xerrors.Errorf("wrong nonce for: %d, expeted %d, got %d", msg.From, ri.val.Nonce, msg.Nonce)
	}
	ri.val.Nonce++
	err := s.saveVal(msg.From, ri.val)
	if err != nil {
		return s.root, err
	}

	if s.version < build.SMTVersion {
		err = s.newRoot(msg.Params)
		if err != nil {
			return s.root, err
		}
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
	case tx.PreDataOrder:
		err := s.createOrder(msg)
		if err != nil {
			return s.root, err
		}
	case tx.AddDataOrder:
		err := s.addSeq(msg)
		if err != nil {
			return s.root, err
		}
	case tx.CommitDataOrder:
		err := s.commitOrder(msg)
		if err != nil {
			return s.root, err
		}
	case tx.SubDataOrder:
		err := s.subOrder(msg)
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
	case tx.ConfirmPostIncome:
		err := s.addPay(msg)
		if err != nil {
			return s.root, err
		}
	case tx.UpdateNet:
		err := s.updateNetAddr(msg)
		if err != nil {
			return s.root, err
		}
	default:
		return s.root, xerrors.Errorf("unsupported method: %d", msg.Method)
	}

	logger.Debug("block apply message: ", msg.From, msg.Method, msg.Nonce, s.root, time.Since(nt))

	return s.root, nil
}
