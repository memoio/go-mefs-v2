package state

import (
	"math/big"

	"github.com/fxamacker/cbor/v2"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

func (s *stateMgr) getOrder(userID, proID uint64) *orderInfo {
	okey := orderKey{
		userID: userID,
		proID:  proID,
	}

	oinfo, ok := s.oInfo[okey]
	if ok {
		return oinfo
	}

	oinfo = &orderInfo{
		Nonce:    0,
		SeqNum:   0,
		Size:     0,
		Price:    big.NewInt(0),
		AccSize:  0,
		AccPrice: big.NewInt(0),
	}

	s.oInfo[okey] = oinfo

	key := store.NewKey(pb.MetaType_ST_OrderBaseKey, userID, proID)
	data, err := s.ds.Get(key)
	if err == nil {
		return oinfo
	}

	err = cbor.Unmarshal(data, oinfo)
	if err != nil {
		return oinfo
	}

	return oinfo
}

func (s *stateMgr) AddOrder(or *types.SignedOrder) error {
	// verify sign

	s.Lock()
	defer s.Unlock()

	oinfo := s.getOrder(or.UserID, or.ProID)

	if or.Nonce != oinfo.Nonce {
		return ErrRes
	}

	oinfo.Nonce++
	// reset
	oinfo.SeqNum = 0
	oinfo.Size = 0
	oinfo.Price = big.NewInt(0)

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
	data, err = cbor.Marshal(oinfo)
	if err != nil {
		return err
	}
	err = s.ds.Put(key, data)
	if err != nil {
		return err
	}

	return nil
}

func (s *stateMgr) AddSeq(so *types.SignedOrderSeq) error {
	// verify sign

	s.Lock()
	defer s.Unlock()

	oinfo := s.getOrder(so.UserID, so.ProID)

	if oinfo.Nonce != so.Nonce+1 {
		return ErrRes
	}

	if oinfo.SeqNum != so.SeqNum {
		return ErrRes
	}

	// verify size and price

	// verify segment
	for _, seg := range so.Segments {
		err := s.AddChunk(so.UserID, seg.BucketID, seg.Start, seg.Length, so.ProID, so.Nonce, seg.ChunkID)
		if err != nil {
			return err
		}
	}

	// update size and price
	oinfo.Size = so.Size
	oinfo.Price = oinfo.Price.Set(so.Price)
	oinfo.SeqNum++

	// save
	key := store.NewKey(pb.MetaType_ST_OrderSeqKey, so.UserID, so.ProID, so.Nonce)
	data, err := so.Serialize()
	if err != nil {
		return err
	}
	err = s.ds.Put(key, data)
	if err != nil {
		return err
	}

	key = store.NewKey(pb.MetaType_ST_OrderBaseKey, so.UserID, so.ProID)
	data, err = cbor.Marshal(oinfo)
	if err != nil {
		return err
	}
	err = s.ds.Put(key, data)
	if err != nil {
		return err
	}

	return nil
}
