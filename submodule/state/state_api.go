package state

import (
	"context"
	"encoding/binary"

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

var _ api.IState = &stateAPI{}

type stateAPI struct {
	*StateMgr
}

func (s *StateMgr) GetRoot(ctx context.Context) types.MsgID {
	s.RLock()
	defer s.RUnlock()

	return s.root
}

func (s *StateMgr) GetHeight(ctx context.Context) uint64 {
	s.RLock()
	defer s.RUnlock()

	return s.height
}

func (s *StateMgr) GetSlot(ctx context.Context) uint64 {
	s.RLock()
	defer s.RUnlock()

	return s.slot
}

func (s *StateMgr) GetMsgNum() uint16 {
	s.RLock()
	defer s.RUnlock()
	return s.msgNum
}

func (s *StateMgr) GetChalEpoch(ctx context.Context) uint64 {
	s.RLock()
	defer s.RUnlock()
	return s.ceInfo.epoch
}

func (s *StateMgr) GetChalEpochInfo(ctx context.Context) *types.ChalEpoch {
	s.RLock()
	defer s.RUnlock()

	return &types.ChalEpoch{
		Epoch: s.ceInfo.current.Epoch,
		Slot:  s.ceInfo.current.Slot,
		Seed:  s.ceInfo.current.Seed,
	}
}

func (s *StateMgr) GetChalEpochInfoAt(ctx context.Context, epoch uint64) (*types.ChalEpoch, error) {
	s.RLock()
	defer s.RUnlock()
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

func (s *StateMgr) GetNonce(ctx context.Context, roleID uint64) uint64 {
	s.RLock()
	defer s.RUnlock()

	ri, ok := s.rInfo[roleID]
	if ok {
		return ri.val.Nonce
	}

	rv := s.loadVal(roleID)

	return rv.Nonce
}

func (s *StateMgr) GetRoleBaseInfo(userID uint64) (*pb.RoleInfo, error) {
	s.RLock()
	defer s.RUnlock()

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

func (s *StateMgr) GetNetInfo(ctx context.Context, roleID uint64) (peer.AddrInfo, error) {
	s.RLock()
	defer s.RUnlock()

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
	s.RLock()
	defer s.RUnlock()

	return s.getThreshold()
}

func (s *StateMgr) GetAllKeepers(ctx context.Context) []uint64 {
	s.RLock()
	defer s.RUnlock()

	key := store.NewKey(pb.MetaType_ST_KeepersKey)
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

func (s *StateMgr) GetPDPPublicKey(ctx context.Context, userID uint64) (pdpcommon.PublicKey, error) {
	s.RLock()
	defer s.RUnlock()

	key := store.NewKey(pb.MetaType_ST_PDPPublicKey, userID)
	data, err := s.ds.Get(key)
	if err != nil {
		return nil, err
	}

	return pdp.DeserializePublicKey(data)
}

func (s *StateMgr) GetProsForUser(ctx context.Context, userID uint64) []uint64 {
	s.RLock()
	defer s.RUnlock()

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

func (s *StateMgr) GetUsersForPro(ctx context.Context, proID uint64) []uint64 {
	s.RLock()
	defer s.RUnlock()

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

func (s *StateMgr) GetAllUsers(ctx context.Context) []uint64 {
	s.RLock()
	defer s.RUnlock()

	key := store.NewKey(pb.MetaType_ST_UsersKey)
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

func (s *StateMgr) GetBucket(ctx context.Context, userID uint64) uint64 {
	s.RLock()
	defer s.RUnlock()

	key := store.NewKey(pb.MetaType_ST_BucketOptKey, userID)
	data, err := s.ds.Get(key)
	if err == nil && len(data) >= 8 {
		return binary.BigEndian.Uint64(data)
	}

	return 0
}

func (s *StateMgr) GetProof(userID, proID, epoch uint64) bool {
	s.RLock()
	defer s.RUnlock()

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

func (s *StateMgr) GetPostIncome(ctx context.Context, userID, proID uint64) *types.PostIncome {
	s.RLock()
	defer s.RUnlock()

	pi := new(types.PostIncome)
	key := store.NewKey(pb.MetaType_ST_SegPayKey, userID, proID)
	data, err := s.ds.Get(key)
	if err == nil {
		err = pi.Deserialize(data)
		if err == nil {
			return pi
		}
	}

	return pi
}

func (s *StateMgr) GetPostIncomeAt(ctx context.Context, userID, proID, epoch uint64) (*types.SignedPostIncome, error) {
	s.RLock()
	defer s.RUnlock()

	spi := new(types.SignedPostIncome)
	key := store.NewKey(pb.MetaType_ST_SegPayKey, userID, proID, epoch)
	data, err := s.ds.Get(key)
	if err == nil {
		err = spi.Deserialize(data)
		if err == nil {
			return spi, nil
		}
	}

	return nil, xerrors.Errorf("not found")
}

func (s *StateMgr) GetOrderState(ctx context.Context, userID, proID uint64) *types.NonceSeq {
	s.RLock()
	defer s.RUnlock()

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
	s.RLock()
	defer s.RUnlock()

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

func (s *StateMgr) GetOrder(userID, proID, nonce uint64) (*types.OrderFull, error) {
	s.RLock()
	defer s.RUnlock()

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

func (s *StateMgr) GetOrderSeq(userID, proID, nonce uint64, seqNum uint32) (*types.SeqFull, error) {
	s.RLock()
	defer s.RUnlock()

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
	s.RLock()
	defer s.RUnlock()

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
