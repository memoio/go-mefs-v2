package state

import (
	"context"
	"encoding/binary"
	"math"

	"github.com/gogo/protobuf/proto"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/crypto/pdp"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

var _ api.IChainState = &stateAPI{}

type stateAPI struct {
	*StateMgr
}

func (s *StateMgr) StateGetInfo(ctx context.Context) (*api.StateInfo, error) {
	s.lk.RLock()
	defer s.lk.RUnlock()

	si := &api.StateInfo{
		Version: s.version,
		Height:  s.height,
		Slot:    s.slot,
		Epoch:   s.ceInfo.epoch,
		Root:    s.root,
		BlockID: s.blkID,
	}

	return si, nil
}

func (s *StateMgr) StateGetChalEpochInfo(ctx context.Context) (*types.ChalEpoch, error) {
	s.lk.RLock()
	defer s.lk.RUnlock()

	return &types.ChalEpoch{
		Epoch: s.ceInfo.current.Epoch,
		Slot:  s.ceInfo.current.Slot,
		Seed:  s.ceInfo.current.Seed,
	}, nil
}

func (s *StateMgr) StateGetChalEpochInfoAt(ctx context.Context, epoch uint64) (*types.ChalEpoch, error) {
	s.lk.RLock()
	defer s.lk.RUnlock()
	ce := new(types.ChalEpoch)
	if epoch >= s.ceInfo.epoch {
		return ce, xerrors.Errorf("epoch expected lower than %d, got %d", s.ceInfo.epoch, epoch)
	}

	key := store.NewKey(pb.MetaType_ST_ChalEpochKey, epoch)
	data, err := s.ds.Get(key)
	if err != nil {
		return ce, err
	}
	err = ce.Deserialize(data)
	if err != nil {
		return ce, err
	}

	return ce, nil
}

func (s *StateMgr) GetRoot(ctx context.Context) types.MsgID {
	s.lk.RLock()
	defer s.lk.RUnlock()

	return s.root
}

func (s *StateMgr) GetHeight(ctx context.Context) uint64 {
	s.lk.RLock()
	defer s.lk.RUnlock()

	return s.height
}

func (s *StateMgr) GetSlot(ctx context.Context) uint64 {
	s.lk.RLock()
	defer s.lk.RUnlock()

	return s.slot
}

func (s *StateMgr) GetBlockID(ctx context.Context) types.MsgID {
	s.lk.RLock()
	defer s.lk.RUnlock()

	return s.blkID
}

func (s *StateMgr) GetChalEpoch(ctx context.Context) uint64 {
	s.lk.RLock()
	defer s.lk.RUnlock()
	return s.ceInfo.epoch
}

func (s *StateMgr) GetBlockIDAt(ctx context.Context, ht uint64) (types.MsgID, error) {
	s.lk.RLock()
	defer s.lk.RUnlock()

	if ht == math.MaxUint64 {
		return s.genesisBlockID, nil
	}

	key := store.NewKey(pb.MetaType_ST_BlockHeightKey, ht)
	data, err := s.ds.Get(key)
	if err != nil {
		return types.MsgIDUndef, err
	}

	return types.FromBytes(data)
}

func (s *StateMgr) StateGetNonce(ctx context.Context, roleID uint64) uint64 {
	s.lk.RLock()
	defer s.lk.RUnlock()

	ri, ok := s.rInfo[roleID]
	if ok {
		return ri.val.Nonce
	}

	rv := s.loadVal(roleID)

	return rv.Nonce
}

func (s *StateMgr) GetRoleBaseInfo(userID uint64) (*pb.RoleInfo, error) {
	s.lk.RLock()
	defer s.lk.RUnlock()

	pri := new(pb.RoleInfo)
	key := store.NewKey(pb.MetaType_ST_RoleBaseKey, userID)
	data, err := s.ds.Get(key)
	if err != nil {
		return pri, err
	}

	err = proto.Unmarshal(data, pri)
	if err != nil {
		return pri, err
	}

	return pri, nil
}

func (s *StateMgr) StateGetNetInfo(ctx context.Context, roleID uint64) (peer.AddrInfo, error) {
	s.lk.RLock()
	defer s.lk.RUnlock()

	res := new(peer.AddrInfo)
	key := store.NewKey(pb.MetaType_ST_NetKey, roleID)
	data, err := s.ds.Get(key)
	if err != nil {
		return *res, err
	}
	err = res.UnmarshalJSON(data)
	return *res, err
}

func (s *StateMgr) GetThreshold(ctx context.Context) int {
	s.lk.RLock()
	defer s.lk.RUnlock()

	return s.getThreshold()
}

func (s *StateMgr) StateGetAllKeepers(ctx context.Context) []uint64 {
	s.lk.RLock()
	defer s.lk.RUnlock()

	res := make([]uint64, 0, len(s.keepers))
	res = append(res, s.keepers...)

	return res
}

func (s *StateMgr) StateGetPDPPublicKey(ctx context.Context, userID uint64) (pdpcommon.PublicKey, error) {
	s.lk.RLock()
	defer s.lk.RUnlock()

	key := store.NewKey(pb.MetaType_ST_PDPPublicKey, userID)
	data, err := s.ds.Get(key)
	if err != nil {
		return nil, err
	}

	return pdp.DeserializePublicKey(data)
}

func (s *StateMgr) StateGetProsAt(ctx context.Context, userID uint64) []uint64 {
	s.lk.RLock()
	defer s.lk.RUnlock()

	key := store.NewKey(pb.MetaType_ST_ProsKey, userID)
	data, err := s.ds.Get(key)
	if err != nil {
		return nil
	}

	res := make([]uint64, len(data)/8)
	for i := 0; i < len(data)/8; i++ {
		res[i] = binary.BigEndian.Uint64(data[8*i : 8*(i+1)])
	}

	return res
}

func (s *StateMgr) StateGetUsersAt(ctx context.Context, proID uint64) []uint64 {
	s.lk.RLock()
	defer s.lk.RUnlock()

	key := store.NewKey(pb.MetaType_ST_UsersKey, proID)
	data, err := s.ds.Get(key)
	if err != nil {
		return nil
	}

	res := make([]uint64, len(data)/8)
	for i := 0; i < len(data)/8; i++ {
		res[i] = binary.BigEndian.Uint64(data[8*i : 8*(i+1)])
	}

	return res
}

func (s *StateMgr) StateGetAllUsers(ctx context.Context) []uint64 {
	s.lk.RLock()
	defer s.lk.RUnlock()

	res := make([]uint64, 0, len(s.users))
	res = append(res, s.users...)

	return res
}

func (s *StateMgr) StateGetAllProviders(ctx context.Context) []uint64 {
	s.lk.RLock()
	defer s.lk.RUnlock()

	res := make([]uint64, 0, len(s.pros))
	res = append(res, s.pros...)

	return res
}

func (s *StateMgr) GetBucket(ctx context.Context, userID uint64) uint64 {
	s.lk.RLock()
	defer s.lk.RUnlock()

	key := store.NewKey(pb.MetaType_ST_BucketOptKey, userID)
	data, err := s.ds.Get(key)
	if err == nil && len(data) >= 8 {
		return binary.BigEndian.Uint64(data)
	}

	return 0
}

func (s *StateMgr) GetProof(userID, proID, epoch uint64) bool {
	s.lk.RLock()
	defer s.lk.RUnlock()

	proved := false
	okey := orderKey{
		userID: userID,
		proID:  proID,
	}

	oinfo, ok := s.oInfo[okey]
	if ok {
		if oinfo.prove > epoch {
			proved = true
		}
		return proved
	}

	key := store.NewKey(pb.MetaType_ST_SegProofKey, userID, proID)
	data, err := s.ds.Get(key)
	if err == nil && len(data) >= 8 {
		if binary.BigEndian.Uint64(data[:8]) > epoch {
			proved = true
		}
	}

	return proved
}

func (s *StateMgr) StateGetPostIncome(ctx context.Context, userID, proID uint64) (*types.PostIncome, error) {
	s.lk.RLock()
	defer s.lk.RUnlock()

	pi := new(types.PostIncome)
	key := store.NewKey(pb.MetaType_ST_SegPayKey, userID, proID)
	data, err := s.ds.Get(key)
	if err == nil {
		err = pi.Deserialize(data)
		if err == nil {
			return pi, nil
		}
	}

	return nil, xerrors.Errorf("not found")
}

func (s *StateMgr) StateGetPostIncomeAt(ctx context.Context, userID, proID, epoch uint64) (*types.PostIncome, error) {
	s.lk.RLock()
	defer s.lk.RUnlock()

	pi := new(types.PostIncome)
	key := store.NewKey(pb.MetaType_ST_SegPayKey, userID, proID, epoch)
	data, err := s.ds.Get(key)
	if err == nil {
		err = pi.Deserialize(data)
		if err == nil {
			return pi, nil
		}
	}

	return nil, xerrors.Errorf("not found")
}

func (s *StateMgr) StateGetAccPostIncomeAt(ctx context.Context, proID, epoch uint64) (*types.AccPostIncome, error) {
	s.lk.RLock()
	defer s.lk.RUnlock()

	pi := new(types.AccPostIncome)
	key := store.NewKey(pb.MetaType_ST_SegPayKey, 0, proID, epoch)
	data, err := s.ds.Get(key)
	if err == nil {
		err = pi.Deserialize(data)
		if err == nil {
			return pi, nil
		}
	}

	return nil, xerrors.Errorf("not found")
}

func (s *StateMgr) StateGetAccPostIncome(ctx context.Context, proID uint64) (*types.SignedAccPostIncome, error) {
	s.lk.RLock()
	defer s.lk.RUnlock()

	pi := new(types.SignedAccPostIncome)
	key := store.NewKey(pb.MetaType_ST_SegPayComfirmKey, proID)
	data, err := s.ds.Get(key)
	if err == nil {
		err = pi.Deserialize(data)
		if err == nil {
			return pi, nil
		}
	}

	return nil, xerrors.Errorf("not found")
}

func (s *StateMgr) StateGetOrderState(ctx context.Context, userID, proID uint64) *types.NonceSeq {
	s.lk.RLock()
	defer s.lk.RUnlock()

	ns := new(types.NonceSeq)
	key := store.NewKey(pb.MetaType_ST_OrderStateKey, userID, proID)
	data, err := s.ds.Get(key)
	if err == nil {
		err = ns.Deserialize(data)
		if err == nil {
			return ns
		}
	}

	return ns
}

func (s *StateMgr) GetOrderStateAt(userID, proID, epoch uint64) *types.NonceSeq {
	s.lk.RLock()
	defer s.lk.RUnlock()

	ns := new(types.NonceSeq)
	key := store.NewKey(pb.MetaType_ST_OrderStateKey, userID, proID, epoch)
	data, err := s.ds.Get(key)
	if err == nil {
		err = ns.Deserialize(data)
		if err == nil {
			return ns
		}
	}

	// load current
	key = store.NewKey(pb.MetaType_ST_OrderStateKey, userID, proID)
	data, err = s.ds.Get(key)
	if err == nil {
		err = ns.Deserialize(data)
		if err == nil {
			return ns
		}
	}

	return ns
}

func (s *StateMgr) StateGetOrder(ctx context.Context, userID, proID, nonce uint64) (*types.OrderFull, error) {
	s.lk.RLock()
	defer s.lk.RUnlock()

	of := new(types.OrderFull)
	key := store.NewKey(pb.MetaType_ST_OrderBaseKey, userID, proID, nonce)
	data, err := s.ds.Get(key)
	if err == nil && len(data) > 0 {
		err = of.Deserialize(data)
		if err == nil {
			return of, nil
		}
	}

	return nil, xerrors.Errorf("not found order: %d, %d, %d", userID, proID, nonce)
}

func (s *StateMgr) StateGetOrderSeq(ctx context.Context, userID, proID, nonce uint64, seqNum uint32) (*types.SeqFull, error) {
	s.lk.RLock()
	defer s.lk.RUnlock()

	sf := new(types.SeqFull)
	key := store.NewKey(pb.MetaType_ST_OrderSeqKey, userID, proID, nonce, seqNum)
	data, err := s.ds.Get(key)
	if err == nil {
		err = sf.Deserialize(data)
		if err == nil {
			return sf, nil
		}
	}

	return nil, xerrors.Errorf("not found order seq:%d, %d, %d, %d ", userID, proID, nonce, seqNum)
}

func (s *StateMgr) GetOrderDuration(userID, proID uint64) *types.OrderDuration {
	s.lk.RLock()
	defer s.lk.RUnlock()

	sf := new(types.OrderDuration)
	key := store.NewKey(pb.MetaType_ST_OrderDurationKey, userID, proID)
	data, err := s.ds.Get(key)
	if err != nil {
		return sf
	}
	err = sf.Deserialize(data)
	if err != nil {
		return sf
	}

	return sf
}
