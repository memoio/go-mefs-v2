package state

import (
	"encoding/binary"
	"math/big"
	"time"

	"github.com/zeebo/blake3"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/build"
	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12_381"
	"github.com/memoio/go-mefs-v2/lib/crypto/pdp"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

func (s *StateMgr) addSegProof(msg *tx.Message, tds store.TxnStore) error {
	nt := time.Now()
	scp := new(tx.SegChalParams)
	err := scp.Deserialize(msg.Params)
	if err != nil {
		return err
	}
	prf, err := pdp.DeserializeProof(scp.Proof)
	if err != nil {
		return err
	}

	if scp.Price == nil {
		return xerrors.Errorf("wrong paras price")
	}

	if msg.From != scp.ProID {
		return xerrors.Errorf("wrong pro expected %d, got %d", msg.From, scp.ProID)
	}

	if scp.Epoch != s.ceInfo.current.Epoch {
		return xerrors.Errorf("wrong challenge epoch, expectd %d, got %d", s.ceInfo.current.Epoch, scp.Epoch)
	}

	okey := orderKey{
		userID: scp.UserID,
		proID:  scp.ProID,
	}

	oinfo, ok := s.oInfo[okey]
	if !ok {
		oinfo = s.loadOrder(okey.userID, okey.proID)
		s.oInfo[okey] = oinfo
	}

	// todo: add penalty if oinfo.prove < scp.Epoch?
	if oinfo.prove > scp.Epoch {
		return xerrors.Errorf("challenge proof submitted or missed at %d", scp.Epoch)
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
	data, err := tds.Get(key)
	if err == nil {
		ns.Deserialize(data)
	}

	if ns.Nonce == 0 && ns.SeqNum == 0 {
		return xerrors.Errorf("challenge on empty data")
	}

	if scp.OrderStart > scp.OrderEnd {
		return xerrors.Errorf("chal has invalid orders %d %d at wrong epoch: %d ", scp.OrderStart, scp.OrderEnd, s.ceInfo.epoch)
	}

	if ns.Nonce != scp.OrderEnd {
		return xerrors.Errorf("chal has wrong order end %d, expected %d at epoch: %d ", scp.OrderEnd, ns.Nonce, s.ceInfo.epoch)
	}

	if ns.SubNonce != scp.OrderStart {
		logger.Debugf("%d chal has wrong order start %d, expected %d at epoch: %d ", scp.OrderStart, ns.SubNonce, s.ceInfo.epoch)
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
	delSize := uint64(0)

	// always challenge latest one
	if ns.SeqNum > 0 {
		// load order
		key = store.NewKey(pb.MetaType_ST_OrderBaseKey, okey.userID, okey.proID, ns.Nonce)
		data, err = tds.Get(key)
		if err != nil {
			return err
		}
		of := new(types.OrderFull)
		err = of.Deserialize(data)
		if err != nil {
			return err
		}

		if of.Start < chalEnd {
			if of.End <= chalStart {
				return xerrors.Errorf("chal order all expired")
			} else if of.Start <= chalStart && of.End >= chalEnd {
				orderDur = chalDur
			} else if of.Start >= chalStart && of.End >= chalEnd {
				orderDur = chalEnd - of.Start
			} else if of.Start <= chalStart && of.End <= chalEnd {
				orderDur = of.End - chalStart
			}

			for i := uint32(0); i < ns.SeqNum; i++ {
				key = store.NewKey(pb.MetaType_ST_OrderSeqKey, okey.userID, okey.proID, ns.Nonce, i)
				data, err = tds.Get(key)
				if err != nil {
					return err
				}
				sf := new(types.SeqFull)
				err = sf.Deserialize(data)
				if err != nil {
					return err
				}

				// calc del price and size
				price.Set(sf.DelPart.Price)
				price.Mul(price, big.NewInt(orderDur))
				totalPrice.Sub(totalPrice, price)
				delSize += sf.DelPart.Size
				chal.Add(bls.SubFr(sf.AccFr, sf.DelPart.AccFr))

				if i == ns.SeqNum-1 {
					totalSize += sf.Size
					price.Set(sf.Price)
					price.Mul(price, big.NewInt(orderDur))
					totalPrice.Add(totalPrice, price)
				}
			}
		}
	}

	if ns.Nonce > 0 {
		// todo: choose some from [0, ns.Nonce)
		for i := scp.OrderStart; i < scp.OrderEnd; i++ {
			key := store.NewKey(pb.MetaType_ST_OrderBaseKey, okey.userID, okey.proID, i)
			data, err = tds.Get(key)
			if err != nil {
				return err
			}
			of := new(types.OrderFull)
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
			price.Sub(price, of.DelPart.Price)
			price.Mul(price, big.NewInt(orderDur))
			totalPrice.Add(totalPrice, price)
			totalSize += of.Size
			delSize += of.DelPart.Size

			chal.Add(bls.SubFr(of.AccFr, of.DelPart.AccFr))
		}
	}

	totalSize -= delSize

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

	logger.Debugf("apply challenge proof: user %d, pro %d, epoch %d, order nonce %d, seqnum %d, size %d, price %d, total income %d, penalty %d, cost %s", okey.userID, okey.proID, scp.Epoch, ns.Nonce, ns.SeqNum, totalSize, totalPrice, oinfo.income.Value, oinfo.income.Penalty, time.Since(nt))

	// save proof result
	key = store.NewKey(pb.MetaType_ST_SegProofKey, okey.userID, okey.proID, scp.Epoch)
	err = tds.Put(key, msg.Params)
	if err != nil {
		return err
	}

	key = store.NewKey(pb.MetaType_ST_SegProofKey, okey.userID, okey.proID)
	buf = make([]byte, 8)
	binary.BigEndian.PutUint64(buf, oinfo.prove)
	err = tds.Put(key, buf)
	if err != nil {
		return err
	}

	// save postincome
	data, err = oinfo.income.Serialize()
	if err != nil {
		return err
	}
	key = store.NewKey(pb.MetaType_ST_SegPayKey, okey.userID, okey.proID)
	err = tds.Put(key, data)
	if err != nil {
		return err
	}

	// save postincome at this epoch
	pi := &types.PostIncome{
		UserID:     okey.userID,
		ProID:      okey.proID,
		TokenIndex: 0,
		Value:      big.NewInt(0),
		Penalty:    big.NewInt(0),
	}
	key = store.NewKey(pb.MetaType_ST_SegPayKey, okey.userID, okey.proID, scp.Epoch)
	data, err = tds.Get(key)
	if err == nil {
		pi.Deserialize(data)
	}
	pi.Value.Add(pi.Value, totalPrice)
	data, err = pi.Serialize()
	if err != nil {
		return err
	}
	err = tds.Put(key, data)
	if err != nil {
		return err
	}

	// save acc income of this pro
	spi := &types.AccPostIncome{
		ProID:      okey.proID,
		TokenIndex: 0,
		Value:      big.NewInt(0),
		Penalty:    big.NewInt(0),
	}
	key = store.NewKey(pb.MetaType_ST_SegPayKey, okey.proID)
	data, err = tds.Get(key)
	if err == nil {
		spi.Deserialize(data)
	}
	spi.Value.Add(spi.Value, totalPrice)
	data, err = spi.Serialize()
	if err != nil {
		return err
	}
	err = tds.Put(key, data)
	if err != nil {
		return err
	}

	key = store.NewKey(pb.MetaType_ST_SegPayKey, 0, okey.proID, scp.Epoch)
	err = tds.Put(key, data)
	if err != nil {
		return err
	}

	// save for next
	key = store.NewKey(pb.MetaType_ST_OrderStateKey, okey.userID, okey.proID, s.ceInfo.epoch)
	data, err = oinfo.ns.Serialize()
	if err != nil {
		return err
	}
	err = tds.Put(key, data)
	if err != nil {
		return err
	}

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

	if scp.Price == nil {
		return xerrors.Errorf("wrong paras price")
	}

	if msg.From != scp.ProID {
		return xerrors.Errorf("wrong pro expected %d, got %d", msg.From, scp.ProID)
	}

	if scp.Epoch != s.validateCeInfo.current.Epoch {
		return xerrors.Errorf("wrong challenge epoch, expectd %d, got %d", s.validateCeInfo.current.Epoch, scp.Epoch)
	}

	okey := orderKey{
		userID: scp.UserID,
		proID:  scp.ProID,
	}

	oinfo, ok := s.validateOInfo[okey]
	if !ok {
		oinfo = s.loadOrder(okey.userID, okey.proID)
		s.validateOInfo[okey] = oinfo
	}

	if oinfo.prove > scp.Epoch {
		return xerrors.Errorf("challenge proof submitted or missed at %d", scp.Epoch)
	}

	buf := make([]byte, 8+len(s.validateCeInfo.current.Seed.Bytes()))
	binary.BigEndian.PutUint64(buf[:8], okey.userID)
	copy(buf[8:], s.validateCeInfo.current.Seed.Bytes())
	bh := blake3.Sum256(buf)

	uinfo, ok := s.validateSInfo[okey.userID]
	if !ok {
		// load pk cost too much time
		uinfo, ok = s.sInfo[okey.userID]
		if !ok {
			uinfo, err = s.loadUser(okey.userID)
			if err != nil {
				return err
			}

			s.validateSInfo[okey.userID] = uinfo
		}
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

	if ns.Nonce != scp.OrderEnd {
		return xerrors.Errorf("chal has wrong order end %d, expected %d at epoch: %d ", scp.OrderEnd, ns.Nonce, s.ceInfo.epoch)
	}

	if ns.SubNonce != scp.OrderStart {
		logger.Debugf("%d chal has wrong order start %d, expected %d at epoch: %d ", scp.OrderStart, ns.SubNonce, s.ceInfo.epoch)
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
	delSize := uint64(0)

	// always challenge latest one
	if ns.SeqNum > 0 {
		// load order
		key = store.NewKey(pb.MetaType_ST_OrderBaseKey, okey.userID, okey.proID, ns.Nonce)
		data, err = s.ds.Get(key)
		if err != nil {
			return err
		}
		of := new(types.OrderFull)
		err = of.Deserialize(data)
		if err != nil {
			return err
		}

		if of.Start < chalEnd {
			if of.End <= chalStart {
				return xerrors.Errorf("chal order all expired")
			} else if of.Start <= chalStart && of.End >= chalEnd {
				orderDur = chalDur
			} else if of.Start >= chalStart && of.End >= chalEnd {
				orderDur = chalEnd - of.Start
			} else if of.Start <= chalStart && of.End <= chalEnd {
				orderDur = of.End - chalStart
			}

			for i := uint32(0); i < ns.SeqNum; i++ {
				key = store.NewKey(pb.MetaType_ST_OrderSeqKey, okey.userID, okey.proID, ns.Nonce, i)
				data, err = s.ds.Get(key)
				if err != nil {
					return err
				}
				sf := new(types.SeqFull)
				err = sf.Deserialize(data)
				if err != nil {
					return err
				}

				// calc del price and size
				price.Set(sf.DelPart.Price)
				price.Mul(price, big.NewInt(orderDur))
				totalPrice.Sub(totalPrice, price)
				delSize += sf.DelPart.Size
				chal.Add(bls.SubFr(sf.AccFr, sf.DelPart.AccFr))

				if i == ns.SeqNum-1 {
					totalSize += sf.Size
					price.Set(sf.Price)
					price.Mul(price, big.NewInt(orderDur))
					totalPrice.Add(totalPrice, price)
				}
			}
		}
	}

	if ns.Nonce > 0 {
		// todo: choose some from [0, ns.Nonce)
		for i := scp.OrderStart; i < scp.OrderEnd; i++ {
			key := store.NewKey(pb.MetaType_ST_OrderBaseKey, okey.userID, okey.proID, i)
			data, err = s.ds.Get(key)
			if err != nil {
				return err
			}
			of := new(types.OrderFull)
			err = of.Deserialize(data)
			if err != nil {
				return err
			}

			if of.Start >= chalEnd {
				continue
			}

			if of.End <= chalStart {
				return xerrors.Errorf("chal order expired at %d", i)
			} else if of.Start <= chalStart && of.End >= chalEnd {
				orderDur = chalDur
			} else if of.Start >= chalStart && of.End >= chalEnd {
				orderDur = chalEnd - of.Start
			} else if of.Start <= chalStart && of.End <= chalEnd {
				orderDur = of.End - chalStart
			}

			price.Set(of.Price)
			price.Sub(price, of.DelPart.Price)
			price.Mul(price, big.NewInt(orderDur))
			totalPrice.Add(totalPrice, price)
			delSize += of.DelPart.Size
			totalSize += of.Size

			chal.Add(bls.SubFr(of.AccFr, of.DelPart.AccFr))
		}
	}

	totalSize -= delSize

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
