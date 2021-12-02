package state

import (
	"encoding/binary"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/crypto/pdp"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"golang.org/x/xerrors"
)

var _ api.IState = &stateAPI{}

type stateAPI struct {
	*StateMgr
}

func (s *StateMgr) GetRoot() types.MsgID {
	s.RLock()
	defer s.RUnlock()

	return s.root
}

func (s *StateMgr) GetHeight() (uint64, uint64, uint16) {
	s.RLock()
	defer s.RUnlock()

	return s.height, s.slot, s.msgNum
}

func (s *StateMgr) GetPublicKey(userID uint64) (pdpcommon.PublicKey, error) {
	key := store.NewKey(pb.MetaType_ST_PDPPublicKey, userID)
	data, err := s.ds.Get(key)
	if err != nil {
		return nil, err
	}

	return pdp.DeserializePublicKey(data)
}

func (s *StateMgr) GetProof(userID, proID, epoch uint64) bool {
	proved := false
	s.RLock()
	okey := orderKey{
		userID: userID,
		proID:  proID,
	}

	oinfo, ok := s.oInfo[okey]
	if ok {
		if oinfo.prove > epoch {
			proved = true
		}
		s.RUnlock()
		return proved
	}
	s.RUnlock()

	key := store.NewKey(pb.MetaType_ST_SegProof, userID, proID)
	data, err := s.ds.Get(key)
	if err == nil && len(data) >= 8 {
		if binary.BigEndian.Uint64(data[:8]) > epoch {
			proved = true
		}
	}

	return proved
}

func (s *StateMgr) GetChalEpoch() uint64 {
	s.RLock()
	defer s.RUnlock()
	return s.chalEpoch
}

func (s *StateMgr) GetChalEpochInfo() *types.ChalEpoch {
	s.RLock()
	defer s.RUnlock()

	return &types.ChalEpoch{
		Epoch: s.chalEpochInfo.Epoch,
		Slot:  s.chalEpochInfo.Slot,
		Seed:  s.chalEpochInfo.Seed,
	}
}

func (s *StateMgr) GetOrderState(userID, proID, epoch uint64) *types.NonceSeq {
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

func (s *StateMgr) GetOrder(userID, proID, nonce uint64) (*types.SignedOrder, []byte, uint32, error) {
	of := new(orderFull)
	key := store.NewKey(pb.MetaType_ST_OrderBaseKey, userID, proID, nonce)
	data, err := s.ds.Get(key)
	if err == nil {
		err = of.Deserialize(data)
		if err == nil {
			return &of.SignedOrder, of.AccFr, of.SeqNum, nil
		}
	}

	return nil, nil, 0, xerrors.Errorf("not found order: %d, %d, %d", userID, proID, nonce)
}

func (s *StateMgr) GetOrderSeq(userID, proID, nonce uint64, seqNum uint32) (*types.OrderSeq, []byte, error) {
	sf := new(seqFull)
	key := store.NewKey(pb.MetaType_ST_OrderSeqKey, userID, proID, nonce, seqNum)
	data, err := s.ds.Get(key)
	if err == nil {
		err = sf.Deserialize(data)
		if err == nil {
			return &sf.OrderSeq, sf.AccFr, nil
		}
	}

	return nil, nil, xerrors.Errorf("not found order seq:%d, %d, %d, %d ", userID, proID, nonce, seqNum)
}
