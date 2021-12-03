package state

import (
	"encoding/binary"
	"math/big"

	"github.com/zeebo/blake3"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/build"
	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12_381"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

// key: pb.MetaType_ST_OrderStateKey/userID/proID; val: types.OrderNonce
// key: pb.MetaType_ST_OrderBaseKey/userID/proID; val: order start and end
// key: pb.MetaType_ST_OrderBaseKey/userID/proID/nonce; val: SignedOrder + accFr
// key: pb.MetaType_ST_OrderSeqKey/userID/proID/nonce/seqNum; val: OrderSeq + accFr

// for challenge last order at some epoch
// key: pb.MetaType_ST_OrderStateKey/userID/proID/epoch; val: types.OrderNonce

// key: pb.MetaType_ST_SegProof/userID/proID; val: proof epoch
// key: pb.MetaType_ST_SegProof/userID/proID/epoch; val: proof

// for chal pay
// key: pb.MetaType_ST_OrderDuration/userID/proID; val: order durations;
func (s *StateMgr) loadOrder(userID, proID uint64) *orderInfo {
	oinfo := &orderInfo{
		ns: &types.NonceSeq{
			Nonce:  0,
			SeqNum: 0,
		},
		accFr: bls.ZERO,
		od:    new(types.OrderDuration),
	}

	// load proof
	key := store.NewKey(pb.MetaType_ST_SegProofKey, userID, proID)
	data, err := s.ds.Get(key)
	if err == nil && len(data) >= 8 {
		oinfo.prove = binary.BigEndian.Uint64(data[:8])
	}

	// load order state
	key = store.NewKey(pb.MetaType_ST_OrderStateKey, userID, proID)
	data, err = s.ds.Get(key)
	if err == nil {
		oinfo.ns.Deserialize(data)
	}

	if oinfo.ns.Nonce == 0 {
		return oinfo
	}

	key = store.NewKey(pb.MetaType_ST_OrderDurationKey, userID, proID)
	data, err = s.ds.Get(key)
	if err == nil {
		oinfo.od.Deserialize(data)
	}

	// load current order
	key = store.NewKey(pb.MetaType_ST_OrderBaseKey, userID, proID, oinfo.ns.Nonce-1)
	data, err = s.ds.Get(key)
	if err != nil {
		return oinfo
	}
	of := new(orderFull)
	err = of.Deserialize(data)
	if err != nil {
		return oinfo
	}
	oinfo.base = &of.SignedOrder
	bls.FrFromBytes(&oinfo.accFr, of.AccFr)

	return oinfo
}

func (s *StateMgr) addOrder(msg *tx.Message) error {
	or := new(types.SignedOrder)
	err := or.Deserialize(msg.Params)
	if err != nil {
		return err
	}

	// todo: verify sign

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
		return xerrors.Errorf("add order nonce wrong, got %d, expected %d", or.Nonce, oinfo.ns.Nonce)
	}

	err = oinfo.od.Add(or.Start, or.End)
	if err != nil {
		return err
	}

	oinfo.ns.Nonce++
	oinfo.base = or
	// reset
	oinfo.ns.SeqNum = 0
	oinfo.accFr = bls.ZERO

	// save order
	key := store.NewKey(pb.MetaType_ST_OrderBaseKey, or.UserID, or.ProID, or.Nonce)
	of := &orderFull{
		SignedOrder: *or,
		SeqNum:      oinfo.ns.SeqNum,
		AccFr:       bls.FrToBytes(&oinfo.accFr),
	}
	data, err := of.Serialize()
	if err != nil {
		return err
	}
	err = s.ds.Put(key, data)
	if err != nil {
		return err
	}

	key = store.NewKey(pb.MetaType_ST_OrderDurationKey, or.UserID, or.ProID)
	data, err = oinfo.od.Serialize()
	if err != nil {
		return err
	}
	err = s.ds.Put(key, data)
	if err != nil {
		return err
	}

	// save state
	key = store.NewKey(pb.MetaType_ST_OrderStateKey, or.UserID, or.ProID)
	data, err = oinfo.ns.Serialize()
	if err != nil {
		return err
	}
	err = s.ds.Put(key, data)
	if err != nil {
		return err
	}

	// save for challenge
	key = store.NewKey(pb.MetaType_ST_OrderStateKey, or.UserID, or.ProID, s.ceInfo.epoch)
	err = s.ds.Put(key, data)
	if err != nil {
		return err
	}

	// callback for user-pro relationgrep fa

	return nil
}

func (s *StateMgr) canAddOrder(msg *tx.Message) error {
	or := new(types.SignedOrder)
	err := or.Deserialize(msg.Params)
	if err != nil {
		return err
	}

	// todo: verify sign

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
		return xerrors.Errorf("add order nonce wrong, got %d, expected %d", or.Nonce, oinfo.ns.Nonce)
	}

	err = oinfo.od.Add(or.Start, or.End)
	if err != nil {
		return err
	}

	oinfo.ns.Nonce++
	oinfo.base = or
	// reset
	oinfo.ns.SeqNum = 0
	oinfo.accFr = bls.ZERO

	return nil
}

func (s *StateMgr) addSeq(msg *tx.Message) error {
	so := new(types.SignedOrderSeq)
	err := so.Deserialize(msg.Params)
	if err != nil {
		return err
	}

	// todo: verify sign

	okey := orderKey{
		userID: so.UserID,
		proID:  so.ProID,
	}

	oinfo, ok := s.oInfo[okey]
	if !ok {
		oinfo = s.loadOrder(okey.userID, okey.proID)
		s.oInfo[okey] = oinfo
	}

	uinfo, ok := s.sInfo[okey.userID]
	if !ok {
		uinfo, err = s.loadUser(okey.userID)
		if err != nil {
			return err
		}
		s.sInfo[okey.userID] = uinfo
	}

	if oinfo.ns.Nonce != so.Nonce+1 {
		return xerrors.Errorf("add seq nonce err got %d, expected %d", so.Nonce, oinfo.ns.Nonce)
	}

	if oinfo.ns.SeqNum != so.SeqNum {
		return xerrors.Errorf("add seq seqnum err got %d, expected %d", so.SeqNum, oinfo.ns.SeqNum)
	}

	// verify size and price
	size := uint64(0)
	for _, seg := range so.Segments {
		size += (seg.Length * build.DefaultSegSize)
	}
	if oinfo.base.Size+size != so.Size {
		return xerrors.Errorf("add seq size wrong, got %d, expected %d", so.Size, oinfo.base.Size+size)
	}

	price := new(big.Int).Mul(oinfo.base.SegPrice, big.NewInt(int64(size)))
	price.Add(price, oinfo.base.Price)
	if price.Cmp(so.Price) != 0 {
		return xerrors.Errorf("add seq price wrong, got %d, expected %d", so.Price, price)
	}

	// verify segment
	for _, seg := range so.Segments {
		err := s.addChunk(so.UserID, seg.BucketID, seg.Start, seg.Length, so.ProID, so.Nonce, seg.ChunkID)
		if err != nil {
			return err
		}
	}

	var HWi bls.Fr
	for _, seg := range so.Segments {
		for i := seg.Start; i < seg.Start+seg.Length; i++ {
			// calculate fr
			sid := segment.CreateSegmentID(uinfo.fsID, seg.BucketID, i, seg.ChunkID)
			h := blake3.Sum256(sid)
			bls.FrFromBytes(&HWi, h[:])
			bls.FrAddMod(&oinfo.accFr, &oinfo.accFr, &HWi)
		}
	}

	// validate size and price
	oinfo.ns.SeqNum++
	oinfo.base.Size = so.Size
	oinfo.base.Price.Set(so.Price)
	oinfo.base.Usign = so.UserSig
	oinfo.base.Psign = so.ProSig

	// save order
	key := store.NewKey(pb.MetaType_ST_OrderBaseKey, so.UserID, so.ProID, oinfo.base.Nonce)
	of := &orderFull{
		SignedOrder: *oinfo.base,
		SeqNum:      oinfo.ns.SeqNum,
		AccFr:       bls.FrToBytes(&oinfo.accFr),
	}
	data, err := of.Serialize()
	if err != nil {
		return err
	}
	err = s.ds.Put(key, data)
	if err != nil {
		return err
	}

	// save seq
	key = store.NewKey(pb.MetaType_ST_OrderSeqKey, so.UserID, so.ProID, so.Nonce, so.SeqNum)
	sf := &seqFull{
		OrderSeq: so.OrderSeq,
		AccFr:    bls.FrToBytes(&oinfo.accFr),
	}
	data, err = sf.Serialize()
	if err != nil {
		return err
	}
	err = s.ds.Put(key, data)
	if err != nil {
		return err
	}

	//save state
	key = store.NewKey(pb.MetaType_ST_OrderStateKey, so.UserID, so.ProID)
	data, err = oinfo.ns.Serialize()
	if err != nil {
		return err
	}
	err = s.ds.Put(key, data)
	if err != nil {
		return err
	}

	key = store.NewKey(pb.MetaType_ST_OrderStateKey, so.UserID, so.ProID, s.ceInfo.epoch)
	err = s.ds.Put(key, data)
	if err != nil {
		return err
	}

	return nil
}

func (s *StateMgr) canAddSeq(msg *tx.Message) error {
	so := new(types.SignedOrderSeq)
	err := so.Deserialize(msg.Params)
	if err != nil {
		return err
	}

	// todo: verify sign

	okey := orderKey{
		userID: so.UserID,
		proID:  so.ProID,
	}

	oinfo, ok := s.validateOInfo[okey]
	if !ok {
		oinfo = s.loadOrder(okey.userID, okey.proID)
		s.validateOInfo[okey] = oinfo
	}

	uinfo, ok := s.validateSInfo[okey.userID]
	if !ok {
		uinfo, err = s.loadUser(okey.userID)
		if err != nil {
			return err
		}
		s.validateSInfo[okey.userID] = uinfo
	}

	if oinfo.ns.Nonce != so.Nonce+1 {
		return xerrors.Errorf("add seq nonce err got %d, expected %d", so.Nonce, oinfo.ns.Nonce)
	}

	if oinfo.ns.SeqNum != so.SeqNum {
		return xerrors.Errorf("add seq seqnum err got %d, expected %d", so.SeqNum, oinfo.ns.SeqNum)
	}

	// verify size and price
	size := uint64(0)
	for _, seg := range so.Segments {
		size += (seg.Length * build.DefaultSegSize)
	}
	if oinfo.base.Size+size != so.Size {
		return xerrors.Errorf("add seq size wrong, got %d, expected %d", so.Size, oinfo.base.Size+size)
	}

	price := new(big.Int).Mul(oinfo.base.SegPrice, big.NewInt(int64(size)))
	price.Add(price, oinfo.base.Price)
	if price.Cmp(so.Price) != 0 {
		return xerrors.Errorf("add seq price wrong, got %d, expected %d", so.Price, price)
	}

	// verify segment
	for _, seg := range so.Segments {
		err := s.canAddChunk(so.UserID, seg.BucketID, seg.Start, seg.Length, so.ProID, so.Nonce, seg.ChunkID)
		if err != nil {
			return err
		}
	}

	var HWi bls.Fr
	for _, seg := range so.Segments {
		for i := seg.Start; i < seg.Start+seg.Length; i++ {
			// calculate fr
			sid := segment.CreateSegmentID(uinfo.fsID, seg.BucketID, i, seg.ChunkID)
			h := blake3.Sum256(sid)
			bls.FrFromBytes(&HWi, h[:])
			bls.FrAddMod(&oinfo.accFr, &oinfo.accFr, &HWi)
		}
	}

	// validate size and price
	oinfo.ns.SeqNum++
	oinfo.base.Size = so.Size
	oinfo.base.Price.Set(so.Price)
	oinfo.base.Usign = so.UserSig
	oinfo.base.Psign = so.ProSig

	return nil
}
