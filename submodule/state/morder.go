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

// todo: commit order
// key: pb.MetaType_ST_OrderCommit/userID/proID; val: order nonce;

func (s *StateMgr) loadOrder(userID, proID uint64) *orderInfo {
	oinfo := &orderInfo{
		ns: &types.NonceSeq{
			Nonce:  0,
			SeqNum: 0,
		},
		income: &types.PostIncome{
			UserID:  userID,
			ProID:   proID,
			Value:   big.NewInt(0),
			Penalty: big.NewInt(0),
		},
		accFr: bls.ZERO,
		od:    new(types.OrderDuration),
	}

	// load pay
	key := store.NewKey(pb.MetaType_ST_SegPayKey, userID, proID)
	data, err := s.ds.Get(key)
	if err == nil {
		oinfo.income.Deserialize(data)
	}

	// load proof
	key = store.NewKey(pb.MetaType_ST_SegProofKey, userID, proID)
	data, err = s.ds.Get(key)
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

	// load order durations
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
	of := new(types.OrderFull)
	err = of.Deserialize(data)
	if err != nil {
		return oinfo
	}
	oinfo.base = &of.SignedOrder
	bls.FrFromBytes(&oinfo.accFr, of.AccFr)

	return oinfo
}

func (s *StateMgr) addOrder(msg *tx.Message, tds store.TxnStore) error {
	or := new(types.SignedOrder)
	err := or.Deserialize(msg.Params)
	if err != nil {
		return err
	}

	if or.SegPrice == nil {
		return xerrors.Errorf("wrong paras seg price")
	}

	if or.PiecePrice == nil {
		return xerrors.Errorf("wrong paras piece price")
	}

	if or.Price == nil {
		return xerrors.Errorf("wrong paras price")
	}

	if or.End-or.Start < build.OrderDuration {
		return xerrors.Errorf("order duration %d is short than %d", or.End-or.Start, build.OrderDuration)
	}

	if msg.From != or.UserID {
		return xerrors.Errorf("wrong user expected %d, got %d", msg.From, or.UserID)
	}

	// todo: verify sign
	uri, ok := s.rInfo[or.UserID]
	if !ok {
		uri = s.loadRole(or.UserID)
		s.rInfo[or.UserID] = uri
	}

	err = verify(uri.base, or.Hash(), or.Usign)
	if err != nil {
		return err
	}

	pri, ok := s.rInfo[or.ProID]
	if !ok {
		pri = s.loadRole(or.ProID)
		s.rInfo[or.ProID] = pri
	}

	err = verify(pri.base, or.Hash(), or.Psign)
	if err != nil {
		return err
	}

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
	of := &types.OrderFull{
		SignedOrder: *or,
		SeqNum:      oinfo.ns.SeqNum,
		AccFr:       bls.FrToBytes(&bls.ZERO),
		DelPart: types.OrderDelPart{
			AccFr: bls.FrToBytes(&bls.ZERO),
			Price: big.NewInt(0),
		},
	}
	data, err := of.Serialize()
	if err != nil {
		return err
	}
	err = tds.Put(key, data)
	if err != nil {
		return err
	}

	key = store.NewKey(pb.MetaType_ST_OrderDurationKey, or.UserID, or.ProID)
	data, err = oinfo.od.Serialize()
	if err != nil {
		return err
	}
	err = tds.Put(key, data)
	if err != nil {
		return err
	}

	// save state
	key = store.NewKey(pb.MetaType_ST_OrderStateKey, or.UserID, or.ProID)
	data, err = oinfo.ns.Serialize()
	if err != nil {
		return err
	}
	err = tds.Put(key, data)
	if err != nil {
		return err
	}

	// save for challenge
	key = store.NewKey(pb.MetaType_ST_OrderStateKey, or.UserID, or.ProID, s.ceInfo.epoch)
	err = tds.Put(key, data)
	if err != nil {
		return err
	}

	// save up at first nonce
	if or.Nonce == 0 {
		key := store.NewKey(pb.MetaType_ST_ProsKey, or.UserID)
		val, _ := tds.Get(key)
		buf := make([]byte, len(val)+8)
		copy(buf[:len(val)], val)
		binary.BigEndian.PutUint64(buf[len(val):len(val)+8], or.ProID)
		tds.Put(key, buf)

		key = store.NewKey(pb.MetaType_ST_UsersKey, or.ProID)
		val, _ = tds.Get(key)
		buf = make([]byte, len(val)+8)
		copy(buf[:len(val)], val)
		binary.BigEndian.PutUint64(buf[len(val):len(val)+8], or.UserID)
		tds.Put(key, buf)

		// callback for user-pro relation
		if s.handleAddUP != nil {
			s.handleAddUP(okey.userID, okey.proID)
		}
	}

	return nil
}

func (s *StateMgr) canAddOrder(msg *tx.Message) error {
	or := new(types.SignedOrder)
	err := or.Deserialize(msg.Params)
	if err != nil {
		return err
	}

	if or.SegPrice == nil {
		return xerrors.Errorf("wrong paras seg price")
	}

	if or.PiecePrice == nil {
		return xerrors.Errorf("wrong paras piece price")
	}

	if or.Price == nil {
		return xerrors.Errorf("wrong paras price")
	}

	if msg.From != or.UserID {
		return xerrors.Errorf("wrong user expected %d, got %d", msg.From, or.UserID)
	}

	if or.End-or.Start < build.OrderDuration {
		return xerrors.Errorf("order duration %d is short than %d", or.End-or.Start, build.OrderDuration)
	}

	// todo: verify sign
	uri, ok := s.validateRInfo[or.UserID]
	if !ok {
		uri = s.loadRole(or.UserID)
		s.validateRInfo[or.UserID] = uri
	}

	err = verify(uri.base, or.Hash(), or.Usign)
	if err != nil {
		return err
	}

	pri, ok := s.validateRInfo[or.ProID]
	if !ok {
		pri = s.loadRole(or.ProID)
		s.validateRInfo[or.ProID] = pri
	}

	err = verify(pri.base, or.Hash(), or.Psign)
	if err != nil {
		return err
	}

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

func (s *StateMgr) addSeq(msg *tx.Message, tds store.TxnStore) error {
	so := new(types.SignedOrderSeq)
	err := so.Deserialize(msg.Params)
	if err != nil {
		return err
	}

	if so.Price == nil {
		return xerrors.Errorf("wrong paras price")
	}

	if msg.From != so.UserID {
		return xerrors.Errorf("wrong user expected %d, got %d", msg.From, so.UserID)
	}

	sHash := so.Hash()

	// todo: verify sign
	uri, ok := s.rInfo[so.UserID]
	if !ok {
		uri = s.loadRole(so.UserID)
		s.rInfo[so.UserID] = uri
	}

	err = verify(uri.base, sHash.Bytes(), so.UserDataSig)
	if err != nil {
		return err
	}

	pri, ok := s.rInfo[so.ProID]
	if !ok {
		pri = s.loadRole(so.ProID)
		s.rInfo[so.ProID] = pri
	}

	err = verify(pri.base, sHash.Bytes(), so.ProDataSig)
	if err != nil {
		return err
	}

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

	nso := &types.SignedOrder{
		OrderBase: oinfo.base.OrderBase,
		Size:      so.Size,
		Price:     price,
	}

	err = verify(uri.base, nso.Hash(), so.UserSig)
	if err != nil {
		return err
	}

	err = verify(pri.base, nso.Hash(), so.ProSig)
	if err != nil {
		return err
	}

	// verify segment
	for _, seg := range so.Segments {
		err := s.addChunk(so.UserID, seg.BucketID, seg.Start, seg.Length, so.ProID, so.Nonce, seg.ChunkID, tds)
		if err != nil {
			return err
		}
	}

	var accFr, HWi bls.Fr
	for _, seg := range so.Segments {
		for i := seg.Start; i < seg.Start+seg.Length; i++ {
			// calculate fr
			sid := segment.CreateSegmentID(uinfo.fsID, seg.BucketID, i, seg.ChunkID)
			h := blake3.Sum256(sid)
			bls.FrFromBytes(&HWi, h[:])
			bls.FrAddMod(&accFr, &accFr, &HWi)
		}
	}

	// add
	oinfo.ns.SeqNum++
	bls.FrAddMod(&oinfo.accFr, &oinfo.accFr, &accFr)
	oinfo.base.Size = so.Size
	oinfo.base.Price.Set(so.Price)
	oinfo.base.Usign = so.UserSig
	oinfo.base.Psign = so.ProSig

	// save order
	key := store.NewKey(pb.MetaType_ST_OrderBaseKey, so.UserID, so.ProID, oinfo.base.Nonce)
	data, err := tds.Get(key)
	if err != nil {
		return err
	}
	of := new(types.OrderFull)
	err = of.Deserialize(data)
	if err != nil {
		return err
	}
	of.SignedOrder = *oinfo.base
	of.SeqNum = oinfo.ns.SeqNum
	of.AccFr = bls.FrToBytes(&oinfo.accFr)
	data, err = of.Serialize()
	if err != nil {
		return err
	}
	err = tds.Put(key, data)
	if err != nil {
		return err
	}

	// save seq
	key = store.NewKey(pb.MetaType_ST_OrderSeqKey, so.UserID, so.ProID, so.Nonce, so.SeqNum)
	sf := &types.SeqFull{
		OrderSeq: so.OrderSeq,
		AccFr:    bls.FrToBytes(&accFr),
		DelPart: types.OrderDelPart{
			AccFr: bls.FrToBytes(&bls.ZERO),
			Price: big.NewInt(0),
		},
	}
	data, err = sf.Serialize()
	if err != nil {
		return err
	}
	err = tds.Put(key, data)
	if err != nil {
		return err
	}

	//save state
	key = store.NewKey(pb.MetaType_ST_OrderStateKey, so.UserID, so.ProID)
	data, err = oinfo.ns.Serialize()
	if err != nil {
		return err
	}
	err = tds.Put(key, data)
	if err != nil {
		return err
	}

	key = store.NewKey(pb.MetaType_ST_OrderStateKey, so.UserID, so.ProID, s.ceInfo.epoch)
	err = tds.Put(key, data)
	if err != nil {
		return err
	}

	// callback for add segmap and delete data in user
	if s.handleAddSeq != nil {
		s.handleAddSeq(so.OrderSeq)
	}

	return nil
}

func (s *StateMgr) canAddSeq(msg *tx.Message) error {
	so := new(types.SignedOrderSeq)
	err := so.Deserialize(msg.Params)
	if err != nil {
		return err
	}

	if so.Price == nil {
		return xerrors.Errorf("wrong paras price")
	}

	if msg.From != so.UserID {
		return xerrors.Errorf("wrong user expected %d, got %d", msg.From, so.UserID)
	}

	// todo: verify sign
	sHash := so.Hash()

	// todo: verify sign
	uri, ok := s.validateRInfo[so.UserID]
	if !ok {
		uri = s.loadRole(so.UserID)
		s.validateRInfo[so.UserID] = uri
	}

	err = verify(uri.base, sHash.Bytes(), so.UserDataSig)
	if err != nil {
		return err
	}

	pri, ok := s.validateRInfo[so.ProID]
	if !ok {
		pri = s.loadRole(so.ProID)
		s.validateRInfo[so.ProID] = pri
	}

	err = verify(pri.base, sHash.Bytes(), so.ProDataSig)
	if err != nil {
		return err
	}

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

	nso := &types.SignedOrder{
		OrderBase: oinfo.base.OrderBase,
		Size:      so.Size,
		Price:     price,
	}

	err = verify(uri.base, nso.Hash(), so.UserSig)
	if err != nil {
		return err
	}

	err = verify(pri.base, nso.Hash(), so.ProSig)
	if err != nil {
		return err
	}

	// verify segment
	for _, seg := range so.Segments {
		err := s.canAddChunk(so.UserID, seg.BucketID, seg.Start, seg.Length, so.ProID, so.Nonce, seg.ChunkID)
		if err != nil {
			return err
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

func (s *StateMgr) removeSeg(msg *tx.Message, tds store.TxnStore) error {
	so := new(tx.SegRemoveParas)
	err := so.Deserialize(msg.Params)
	if err != nil {
		return err
	}

	if msg.From != so.ProID {
		return xerrors.Errorf("wrong pro expected %d, got %d", msg.From, so.ProID)
	}

	okey := orderKey{
		userID: so.UserID,
		proID:  so.ProID,
	}

	oinfo, ok := s.oInfo[okey]
	if !ok {
		oinfo = s.loadOrder(okey.userID, okey.proID)
		s.oInfo[okey] = oinfo
	}

	uinfo, ok := s.sInfo[so.UserID]
	if !ok {
		uinfo, err = s.loadUser(so.UserID)
		if err != nil {
			return err
		}
		s.sInfo[so.UserID] = uinfo
	}

	// load order seq
	key := store.NewKey(pb.MetaType_ST_OrderBaseKey, so.UserID, so.ProID, so.Nonce)
	data, err := tds.Get(key)
	if err != nil {
		return err
	}
	of := new(types.OrderFull)
	err = of.Deserialize(data)
	if err != nil {
		return err
	}

	stime := build.BaseTime + int64(s.ceInfo.previous.Slot*build.SlotDuration)
	if stime > of.End {
		return xerrors.Errorf("wrong time expected before %d, got %d at nonce %d", of.End, stime, so.Nonce)
	}

	etime := build.BaseTime + int64(s.ceInfo.current.Slot*build.SlotDuration)
	if etime < of.Start {
		return xerrors.Errorf("wrong time expected after %d, got %d at nonce %d", of.Start, etime, so.Nonce)
	}

	key = store.NewKey(pb.MetaType_ST_OrderSeqKey, so.UserID, so.ProID, so.Nonce, so.SeqNum)
	data, err = tds.Get(key)
	if err != nil {
		return err
	}
	sf := new(types.SeqFull)
	err = sf.Deserialize(data)
	if err != nil {
		return err
	}

	var HWi, accFr bls.Fr
	size := uint64(0)
	for _, lseg := range so.Segments {
		for i := lseg.Start; i < lseg.Start+lseg.Length; i++ {
			if !sf.Segments.Has(lseg.BucketID, i, lseg.ChunkID) {
				return xerrors.Errorf("seg %d_%d_%d is not found in seq", lseg.BucketID, i, lseg.ChunkID)
			}
			sid := segment.CreateSegmentID(uinfo.fsID, lseg.BucketID, i, lseg.ChunkID)
			h := blake3.Sum256(sid)
			bls.FrFromBytes(&HWi, h[:])
			bls.FrAddMod(&accFr, &accFr, &HWi)
		}
		size += (lseg.Length * build.DefaultSegSize)
		sf.DelSegs.Push(lseg)
	}

	price := big.NewInt(int64(size))
	price.Mul(price, of.SegPrice)

	penalty := new(big.Int).Mul(price, big.NewInt(of.End-stime))
	oinfo.income.Penalty.Add(oinfo.income.Penalty, penalty)

	// save order
	of.DelPart.Price.Add(of.DelPart.Price, price)
	of.DelPart.Size += size
	bls.FrFromBytes(&HWi, of.DelPart.AccFr)
	bls.FrAddMod(&HWi, &HWi, &accFr)
	of.DelPart.AccFr = bls.FrToBytes(&HWi)

	data, err = of.Serialize()
	if err != nil {
		return err
	}
	key = store.NewKey(pb.MetaType_ST_OrderBaseKey, so.UserID, so.ProID, so.Nonce)
	tds.Put(key, data)

	// save seq
	sf.DelPart.Price.Add(sf.DelPart.Price, price)
	sf.DelPart.Size += size
	bls.FrFromBytes(&HWi, sf.DelPart.AccFr)
	bls.FrAddMod(&HWi, &HWi, &accFr)
	sf.DelPart.AccFr = bls.FrToBytes(&HWi)

	sf.DelSegs.Merge()

	data, err = sf.Serialize()
	if err != nil {
		return err
	}
	key = store.NewKey(pb.MetaType_ST_OrderSeqKey, so.UserID, so.ProID, so.Nonce, so.SeqNum)
	tds.Put(key, data)

	// save post income
	key = store.NewKey(pb.MetaType_ST_SegPayKey, so.UserID, so.ProID)
	data, err = oinfo.income.Serialize()
	if err != nil {
		return err
	}
	tds.Put(key, data)

	// save penalty at epoch
	pi := &types.PostIncome{
		UserID:  so.UserID,
		ProID:   so.ProID,
		Value:   big.NewInt(0),
		Penalty: big.NewInt(0),
	}
	key = store.NewKey(pb.MetaType_ST_SegPayKey, so.UserID, so.ProID, s.ceInfo.current.Epoch)
	data, err = tds.Get(key)
	if err == nil {
		pi.Deserialize(data)
	}
	pi.Penalty.Add(pi.Penalty, penalty)
	data, err = pi.Serialize()
	if err != nil {
		return err
	}
	tds.Put(key, data)

	// save acc
	spi := &types.AccPostIncome{
		ProID:   okey.proID,
		Value:   big.NewInt(0),
		Penalty: big.NewInt(0),
	}
	key = store.NewKey(pb.MetaType_ST_SegPayKey, okey.proID)
	data, err = tds.Get(key)
	if err == nil {
		spi.Deserialize(data)
	}
	spi.Penalty.Add(spi.Penalty, penalty)
	data, err = spi.Serialize()
	if err != nil {
		return err
	}
	tds.Put(key, data)

	key = store.NewKey(pb.MetaType_ST_SegPayKey, 0, okey.proID, s.ceInfo.current.Epoch)
	tds.Put(key, data)

	if s.handleDelSeg != nil {
		s.handleDelSeg(so)
	}

	return nil
}

func (s *StateMgr) canRemoveSeg(msg *tx.Message) error {
	so := new(tx.SegRemoveParas)
	err := so.Deserialize(msg.Params)
	if err != nil {
		return err
	}

	if msg.From != so.ProID {
		return xerrors.Errorf("wrong pro expected %d, got %d", msg.From, so.ProID)
	}

	// load order
	key := store.NewKey(pb.MetaType_ST_OrderBaseKey, so.UserID, so.ProID, so.Nonce)
	data, err := s.ds.Get(key)
	if err != nil {
		return err
	}
	of := new(types.OrderFull)
	err = of.Deserialize(data)
	if err != nil {
		return err
	}

	stime := build.BaseTime + int64(s.validateCeInfo.previous.Slot*build.SlotDuration)
	if stime > of.End {
		return xerrors.Errorf("wrong time expected before %d, got %d at nonce %d", of.End, stime, so.Nonce)
	}

	etime := build.BaseTime + int64(s.validateCeInfo.current.Slot*build.SlotDuration)
	if etime < of.Start {
		return xerrors.Errorf("wrong time expected after %d, got %d at nonce %d", of.Start, etime, so.Nonce)
	}

	// load order seq
	key = store.NewKey(pb.MetaType_ST_OrderSeqKey, so.UserID, so.ProID, so.Nonce, so.SeqNum)
	data, err = s.ds.Get(key)
	if err != nil {
		return err
	}
	sf := new(types.SeqFull)
	err = sf.Deserialize(data)
	if err != nil {
		return err
	}

	for _, lseg := range so.Segments {
		for i := lseg.Start; i < lseg.Start+lseg.Length; i++ {
			if !sf.Segments.Has(lseg.BucketID, i, lseg.ChunkID) {
				return xerrors.Errorf("seg %d_%d_%d is not found in seq", lseg.BucketID, i, lseg.ChunkID)
			}
		}
	}

	return nil
}
