package state

import (
	"github.com/fxamacker/cbor/v2"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

func (s *StateMgr) loadOrder(userID, proID uint64) *orderInfo {
	oinfo := &orderInfo{
		ns: &types.NonceSeq{
			Nonce:  0,
			SeqNum: 0,
		},
	}

	key := store.NewKey(pb.MetaType_ST_OrderBaseKey, userID, proID)
	data, err := s.ds.Get(key)
	if err != nil {
		return oinfo
	}

	err = cbor.Unmarshal(data, oinfo.ns)
	if err != nil {
		return oinfo
	}

	if oinfo.ns.Nonce == 0 {
		return oinfo
	}

	key = store.NewKey(pb.MetaType_ST_OrderBaseKey, userID, proID, oinfo.ns.Nonce-1)
	data, err = s.ds.Get(key)
	if err != nil {
		return oinfo
	}
	so := new(types.SignedOrder)
	err = so.Deserialize(data)
	if err != nil {
		return oinfo
	}
	oinfo.base = so

	return oinfo
}

func (s *StateMgr) AddOrder(or *types.SignedOrder) error {
	// verify sign

	s.Lock()
	defer s.Unlock()

	okey := orderKey{
		userID: or.UserID,
		proID:  or.ProID,
	}

	oinfo, ok := s.oInfo[okey]
	if !ok {
		oinfo = s.loadOrder(or.UserID, or.ProID)
		s.oInfo[okey] = oinfo
	}

	if or.Nonce != oinfo.ns.Nonce {
		return xerrors.Errorf("add order got %d, expected %d, err: %w", or.Nonce, oinfo.ns.Nonce, ErrNonce)
	}

	oinfo.ns.Nonce++
	oinfo.base = or
	// reset
	oinfo.ns.SeqNum = 0

	// save
	key := store.NewKey(pb.MetaType_ST_OrderBaseKey, or.UserID, or.ProID, or.Nonce)
	data, err := or.Serialize()
	if err != nil {
		return err
	}
	err = s.ds.Put(key, data)
	if err != nil {
		return err
	}

	key = store.NewKey(pb.MetaType_ST_OrderBaseKey, or.UserID, or.ProID)
	data, err = cbor.Marshal(oinfo.ns)
	if err != nil {
		return err
	}
	err = s.ds.Put(key, data)
	if err != nil {
		return err
	}

	return nil
}

func (s *StateMgr) AddSeq(so *types.SignedOrderSeq) error {
	// verify sign

	s.Lock()
	defer s.Unlock()

	okey := orderKey{
		userID: so.UserID,
		proID:  so.ProID,
	}

	oinfo, ok := s.oInfo[okey]
	if !ok {
		oinfo = s.loadOrder(so.UserID, so.ProID)
		s.oInfo[okey] = oinfo
	}

	if oinfo.ns.Nonce != so.Nonce+1 {
		return xerrors.Errorf("add seq got %d, expected %d, err: %w", so.Nonce, oinfo.ns.Nonce, ErrNonce)
	}

	if oinfo.ns.SeqNum != so.SeqNum {
		return xerrors.Errorf("add seq got %d, expected %d, err: %w", so.SeqNum, oinfo.ns.SeqNum, ErrSeq)
	}

	// verify size and price

	// verify segment
	for _, seg := range so.Segments {
		err := s.AddChunk(so.UserID, seg.BucketID, seg.Start, seg.Length, so.ProID, so.Nonce, seg.ChunkID)
		if err != nil {
			return err
		}
	}

	// validate size and price
	oinfo.ns.SeqNum++
	oinfo.base.Size = so.Size
	oinfo.base.Price.Set(so.Price)

	// save
	key := store.NewKey(pb.MetaType_ST_OrderBaseKey, so.UserID, so.ProID, oinfo.base.Nonce)
	data, err := oinfo.base.Serialize()
	if err != nil {
		return err
	}
	err = s.ds.Put(key, data)
	if err != nil {
		return err
	}

	key = store.NewKey(pb.MetaType_ST_OrderSeqKey, so.UserID, so.ProID, so.Nonce, so.SeqNum)
	data, err = so.Serialize()
	if err != nil {
		return err
	}
	err = s.ds.Put(key, data)
	if err != nil {
		return err
	}

	key = store.NewKey(pb.MetaType_ST_OrderBaseKey, so.UserID, so.ProID)
	data, err = cbor.Marshal(oinfo.ns)
	if err != nil {
		return err
	}
	err = s.ds.Put(key, data)
	if err != nil {
		return err
	}

	return nil
}

func (s *StateMgr) CanAddOrder(or *types.SignedOrder) error {
	// verify sign

	s.Lock()
	defer s.Unlock()

	okey := orderKey{
		userID: or.UserID,
		proID:  or.ProID,
	}

	oinfo, ok := s.validateOInfo[okey]
	if !ok {
		oinfo = s.loadOrder(or.UserID, or.ProID)
		s.validateOInfo[okey] = oinfo
	}

	if or.Nonce != oinfo.ns.Nonce {
		return xerrors.Errorf("add order got %d, expected %d, err: %w", or.Nonce, oinfo.ns.Nonce, ErrNonce)
	}

	oinfo.ns.Nonce++
	// reset
	oinfo.ns.SeqNum = 0

	return nil
}

func (s *StateMgr) CanAddSeq(so *types.SignedOrderSeq) error {
	// verify sign

	s.Lock()
	defer s.Unlock()

	okey := orderKey{
		userID: so.UserID,
		proID:  so.ProID,
	}

	oinfo, ok := s.validateOInfo[okey]
	if !ok {
		oinfo = s.loadOrder(so.UserID, so.ProID)
		s.validateOInfo[okey] = oinfo
	}

	if oinfo.ns.Nonce != so.Nonce+1 {
		return xerrors.Errorf("add seq got %d, expected %d, err: %w", so.Nonce, oinfo.ns.Nonce, ErrNonce)
	}

	if oinfo.ns.SeqNum != so.SeqNum {
		return xerrors.Errorf("add seq got %d, expected %d, err: %w", so.SeqNum, oinfo.ns.SeqNum, ErrSeq)
	}
	// verify size and price

	// verify segment
	for _, seg := range so.Segments {
		err := s.CanAddChunk(so.UserID, seg.BucketID, seg.Start, seg.Length, so.ProID, so.Nonce, seg.ChunkID)
		if err != nil {
			return err
		}
	}

	// update size and price
	oinfo.ns.SeqNum++

	return nil
}
