package state

import (
	"encoding/binary"
	"math/big"

	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/crypto/pdp"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/zeebo/blake3"
	"golang.org/x/xerrors"
)

func (s *StateMgr) addSegProof(msg *tx.Message) error {
	scp := new(tx.SegChalParams)
	err := scp.Deserialize(msg.Params)
	if err != nil {
		return err
	}
	prf, err := pdp.DeserializeProof(scp.Proof)
	if err != nil {
		return err
	}

	if scp.Epoch != s.ceInfo.current.Epoch {
		return xerrors.Errorf("wrong challenge epoch, expectd %d, got %d", s.ceInfo.current.Epoch, scp.Epoch)
	}

	okey := orderKey{
		userID: msg.To,
		proID:  msg.From,
	}

	oinfo, ok := s.oInfo[okey]
	if !ok {
		oinfo = s.loadOrder(okey.userID, okey.proID)
		s.oInfo[okey] = oinfo
	}

	if oinfo.prove > scp.Epoch {
		return xerrors.Errorf("challeng proof submitted or missed at %d", scp.Epoch)
	}

	buf := make([]byte, 8+len(s.ceInfo.current.Seed.Bytes()))
	binary.BigEndian.PutUint64(buf[:8], okey.userID)
	copy(buf[8:], s.ceInfo.current.Seed.Bytes())
	bh := blake3.Sum256(buf)

	uinfo, ok := s.sInfo[okey.userID]
	if !ok {
		uinfo, err = s.loadUser(okey.userID)
		if err != nil {
			return err
		}
		s.sInfo[okey.userID] = uinfo
	}

	chal, err := pdp.NewChallenge(uinfo.verifyKey, bh)
	if err != nil {
		return err
	}

	// load
	ns := &types.NonceSeq{
		Nonce:  oinfo.ns.Nonce,
		SeqNum: oinfo.ns.SeqNum,
	}
	key := store.NewKey(pb.MetaType_ST_OrderStateKey, okey.userID, okey.proID, scp.Epoch)
	data, err := s.ds.Get(key)
	if err == nil {
		ns.Deserialize(data)
	}

	if ns.Nonce == 0 && ns.SeqNum == 0 {
		return xerrors.Errorf("challenge on empty data")
	}

	if scp.OrderStart > scp.OrderEnd {
		return xerrors.Errorf("chal has invalid orders %d %d at wrong epoch: %d ", scp.OrderStart, scp.OrderEnd, s.ceInfo.epoch)
	}

	if ns.Nonce != scp.OrderEnd+1 {
		return xerrors.Errorf("chal has wrong orders %d, expected %d at epoch: %d ", scp.OrderEnd, ns.Nonce-1, s.ceInfo.epoch)
	}

	chalStart := build.BaseTime + int64(s.ceInfo.previous.Slot*build.SlotDuration)
	chalEnd := build.BaseTime + int64(s.ceInfo.current.Slot*build.SlotDuration)
	chalDur := chalEnd - chalStart
	if chalDur <= 0 {
		return xerrors.Errorf("chal at wrong epoch: %d ", s.ceInfo.epoch)
	}

	orderDur := int64(0)

	price := new(big.Int)
	totalPrice := new(big.Int)
	totalSize := uint64(0)

	// always challenge latest one
	if ns.Nonce > 0 {
		// load order
		key = store.NewKey(pb.MetaType_ST_OrderBaseKey, okey.userID, okey.proID, ns.Nonce-1)
		data, err = s.ds.Get(key)
		if err != nil {
			return err
		}
		of := new(orderFull)
		err = of.Deserialize(data)
		if err != nil {
			return err
		}

		if of.Start >= chalEnd || of.End <= chalStart {
			return xerrors.Errorf("chal order all expired")
		} else if of.Start <= chalStart && of.End >= chalEnd {
			orderDur = chalDur
		} else if of.Start >= chalStart && of.End >= chalEnd {
			orderDur = chalEnd - of.Start
		} else if of.Start <= chalStart && of.End <= chalEnd {
			orderDur = of.End - chalStart
		}

		if ns.SeqNum > 0 {
			key = store.NewKey(pb.MetaType_ST_OrderSeqKey, okey.userID, okey.proID, ns.Nonce-1, ns.SeqNum-1)
			data, err = s.ds.Get(key)
			if err != nil {
				return err
			}
			sf := new(seqFull)
			err = sf.Deserialize(data)
			if err != nil {
				return err
			}
			price.Set(sf.Price)
			price.Mul(price, big.NewInt(orderDur))
			totalPrice.Add(totalPrice, price)
			totalSize += sf.Size
			chal.Add(sf.AccFr)
		}
	}

	if ns.Nonce > 1 {
		// todo: choose some from [0, ns.Nonce-1)
		for i := scp.OrderStart; i < scp.OrderEnd; i++ {
			key := store.NewKey(pb.MetaType_ST_OrderBaseKey, okey.userID, okey.proID, i)
			data, err = s.ds.Get(key)
			if err != nil {
				return err
			}
			of := new(orderFull)
			err = of.Deserialize(data)
			if err != nil {
				return err
			}
			if of.Start >= chalEnd || of.End <= chalStart {
				return xerrors.Errorf("chal order expired at %d", i)
			} else if of.Start <= chalStart && of.End >= chalEnd {
				orderDur = chalDur
			} else if of.Start >= chalStart && of.End >= chalEnd {
				orderDur = chalEnd - of.Start
			} else if of.Start <= chalStart && of.End <= chalEnd {
				orderDur = of.End - chalStart
			}
			price.Set(of.Price)
			price.Mul(price, big.NewInt(orderDur))
			totalPrice.Add(totalPrice, price)
			totalSize += of.Size
			chal.Add(of.AccFr)
		}
	}

	if totalSize != scp.Size {
		return xerrors.Errorf("wrong challenge proof: %d %d %d %d %d, size expected: %d, got %d", okey.userID, okey.proID, scp.Epoch, ns.Nonce, ns.SeqNum, totalSize, scp.Size)
	}

	if totalPrice.Cmp(scp.Price) != 0 {
		return xerrors.Errorf("wrong challenge proof: %d %d %d %d %d, price expected: %d, got %d", okey.userID, okey.proID, scp.Epoch, ns.Nonce, ns.SeqNum, totalPrice, scp.Price)
	}

	ok, err = uinfo.verifyKey.VerifyProof(chal, prf)
	if err != nil {
		return err
	}

	if !ok {
		return xerrors.Errorf("wrong challenge proof: %d %d %d %d %d", okey.userID, okey.proID, scp.Epoch, ns.Nonce, ns.SeqNum)
	}

	oinfo.prove = scp.Epoch + 1

	oinfo.income.Value.Add(oinfo.income.Value, totalPrice)

	logger.Debugf("apply challenge proof: %d %d %d %d %d %d %d", okey.userID, okey.proID, scp.Epoch, ns.Nonce, ns.SeqNum, totalSize, totalPrice)

	// save proof result
	key = store.NewKey(pb.MetaType_ST_SegProofKey, okey.userID, okey.proID, scp.Epoch)
	s.ds.Put(key, msg.Params)

	key = store.NewKey(pb.MetaType_ST_SegProofKey, okey.userID, okey.proID)
	buf = make([]byte, 8)
	binary.BigEndian.PutUint64(buf, oinfo.prove)
	s.ds.Put(key, buf)

	// save posincome
	data, err = oinfo.income.Serialize()
	if err != nil {
		return err
	}
	key = store.NewKey(pb.MetaType_ST_SegPayKey, okey.userID, okey.proID)
	s.ds.Put(key, data)
	// save at epoch
	key = store.NewKey(pb.MetaType_ST_SegPayKey, okey.userID, okey.proID, scp.Epoch)
	s.ds.Put(key, data)

	// keeper handle callback income
	if s.handleAddPay != nil {
		s.handleAddPay(okey.userID, okey.proID, scp.Epoch, oinfo.income.Value, oinfo.income.Penalty)
	}

	return nil
}

func (s *StateMgr) canAddSegProof(msg *tx.Message) error {
	scp := new(tx.SegChalParams)
	err := scp.Deserialize(msg.Params)
	if err != nil {
		return err
	}
	prf, err := pdp.DeserializeProof(scp.Proof)
	if err != nil {
		return err
	}

	if scp.Epoch != s.validateCeInfo.current.Epoch {
		return xerrors.Errorf("wrong challenge epoch, expectd %d, got %d", s.validateCeInfo.current.Epoch, scp.Epoch)
	}

	okey := orderKey{
		userID: msg.To,
		proID:  msg.From,
	}

	oinfo, ok := s.validateOInfo[okey]
	if !ok {
		oinfo = s.loadOrder(okey.userID, okey.proID)
		s.validateOInfo[okey] = oinfo
	}

	if oinfo.prove > scp.Epoch {
		return xerrors.Errorf("challeng proof submitted or missed at %d", scp.Epoch)
	}

	buf := make([]byte, 8+len(s.validateCeInfo.current.Seed.Bytes()))
	binary.BigEndian.PutUint64(buf[:8], okey.userID)
	copy(buf[8:], s.validateCeInfo.current.Seed.Bytes())
	bh := blake3.Sum256(buf)

	uinfo, ok := s.validateSInfo[okey.userID]
	if !ok {
		uinfo, err = s.loadUser(okey.userID)
		if err != nil {
			return err
		}
		s.validateSInfo[okey.userID] = uinfo
	}

	chal, err := pdp.NewChallenge(uinfo.verifyKey, bh)
	if err != nil {
		return err
	}

	ns := &types.NonceSeq{
		Nonce:  oinfo.ns.Nonce,
		SeqNum: oinfo.ns.SeqNum,
	}

	// load
	key := store.NewKey(pb.MetaType_ST_OrderStateKey, okey.userID, okey.proID, scp.Epoch)
	data, err := s.ds.Get(key)
	if err == nil {
		ns.Deserialize(data)
	}

	if ns.Nonce == 0 && ns.SeqNum == 0 {
		return xerrors.Errorf("challenge on empty data")
	}

	if scp.OrderStart > scp.OrderEnd {
		return xerrors.Errorf("chal has invalid orders %d %d at wrong epoch: %d ", scp.OrderStart, scp.OrderEnd, s.ceInfo.epoch)
	}

	if ns.Nonce != scp.OrderEnd+1 {
		return xerrors.Errorf("chal has wrong orders %d, expected %d at epoch: %d ", scp.OrderEnd, ns.Nonce-1, s.ceInfo.epoch)
	}

	chalStart := build.BaseTime + int64(s.validateCeInfo.previous.Slot*build.SlotDuration)
	chalEnd := build.BaseTime + int64(s.validateCeInfo.current.Slot*build.SlotDuration)
	chalDur := chalEnd - chalStart
	if chalDur <= 0 {
		return xerrors.Errorf("chal at wrong epoch: %d ", s.ceInfo.epoch)
	}

	orderDur := int64(0)

	price := new(big.Int)
	totalPrice := new(big.Int)
	totalSize := uint64(0)

	if ns.Nonce > 0 {
		// load order
		key = store.NewKey(pb.MetaType_ST_OrderBaseKey, okey.userID, okey.proID, ns.Nonce-1)
		data, err = s.ds.Get(key)
		if err != nil {
			return err
		}
		of := new(orderFull)
		err = of.Deserialize(data)
		if err != nil {
			return err
		}

		if of.Start >= chalEnd || of.End <= chalStart {
			return xerrors.Errorf("chal order all expired")
		} else if of.Start <= chalStart && of.End >= chalEnd {
			orderDur = chalDur
		} else if of.Start <= chalStart && of.End <= chalEnd {
			orderDur = of.End - chalStart
		} else if of.Start >= chalStart && of.End >= chalEnd {
			orderDur = chalEnd - of.Start
		} else if of.Start >= chalStart && of.End <= chalEnd {
			orderDur = of.End - of.Start
		}

		if ns.SeqNum > 0 {
			key = store.NewKey(pb.MetaType_ST_OrderSeqKey, okey.userID, okey.proID, ns.Nonce-1, ns.SeqNum-1)
			data, err = s.ds.Get(key)
			if err != nil {
				return err
			}
			sf := new(seqFull)
			err = sf.Deserialize(data)
			if err != nil {
				return err
			}

			price.Set(sf.Price)
			price.Mul(price, big.NewInt(orderDur))
			totalPrice.Add(totalPrice, price)
			totalSize += sf.Size
			chal.Add(sf.AccFr)
		}
	}

	if ns.Nonce > 1 {
		// todo: choose some un-expire orders from [scp.OrderStart, ns.Nonce-1)
		for i := scp.OrderStart; i < scp.OrderEnd; i++ {
			key := store.NewKey(pb.MetaType_ST_OrderBaseKey, okey.userID, okey.proID, i)
			data, err = s.ds.Get(key)
			if err != nil {
				return err
			}
			of := new(orderFull)
			err = of.Deserialize(data)
			if err != nil {
				return err
			}

			if of.Start >= chalEnd || of.End <= chalStart {
				return xerrors.Errorf("chal order expired at %d", i)
			} else if of.Start <= chalStart && of.End >= chalEnd {
				orderDur = chalDur
			} else if of.Start <= chalStart && of.End <= chalEnd {
				orderDur = of.End - chalStart
			} else if of.Start >= chalStart && of.End >= chalEnd {
				orderDur = chalEnd - of.Start
			} else if of.Start >= chalStart && of.End <= chalEnd {
				orderDur = of.End - of.Start
			}

			price.Set(of.Price)
			price.Mul(price, big.NewInt(orderDur))
			totalPrice.Add(totalPrice, price)
			totalSize += of.Size
			chal.Add(of.AccFr)
		}
	}

	if totalSize != scp.Size {
		return xerrors.Errorf("wrong challenge proof: %d %d %d %d %d, size expected: %d, got %d", okey.userID, okey.proID, scp.Epoch, ns.Nonce, ns.SeqNum, totalSize, scp.Size)
	}

	if totalPrice.Cmp(scp.Price) != 0 {
		return xerrors.Errorf("wrong challenge proof: %d %d %d %d %d, price expected: %d, got %d", okey.userID, okey.proID, scp.Epoch, ns.Nonce, ns.SeqNum, totalPrice, scp.Price)
	}

	ok, err = uinfo.verifyKey.VerifyProof(chal, prf)
	if err != nil {
		return err
	}

	if !ok {
		return xerrors.Errorf("wrong challenge proof: %d %d %d %d %d", okey.userID, okey.proID, scp.Epoch, ns.Nonce, ns.SeqNum)
	}

	logger.Debugf("validate challenge proof: %d %d %d %d %d %d %d", okey.userID, okey.proID, scp.Epoch, ns.Nonce, ns.SeqNum, totalSize, totalPrice)

	oinfo.prove = scp.Epoch + 1

	return nil
}
