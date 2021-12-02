package state

import (
	"encoding/binary"

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

	if scp.Epoch != s.chalEpochInfo.Epoch {
		return xerrors.Errorf("wrong challenge epoch, expectd %d, got %d", s.chalEpochInfo.Epoch, scp.Epoch)
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

	buf := make([]byte, 8+len(s.chalEpochInfo.Seed.Bytes()))
	binary.BigEndian.PutUint64(buf[:8], okey.userID)
	copy(buf[8:], s.chalEpochInfo.Seed.Bytes())
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
	key := store.NewKey(pb.MetaType_ST_OrderStateKey, okey.userID, okey.proID)
	data, err := s.ds.Get(key)
	if err == nil {
		ns.Deserialize(data)
	}

	if ns.Nonce == 0 && ns.SeqNum == 0 {
		return xerrors.Errorf("challenge on empty data")
	}

	// always challenge latest one
	if ns.Nonce > 0 {
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

			chal.Add(sf.AccFr)
		} else {
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
			if of.SeqNum > 0 {
				chal.Add(of.AccFr)
			}
		}
	}

	if ns.Nonce > 1 {
		// todo: choose some from [0, ns.Nonce-1)
		for i := uint64(0); i < ns.Nonce-1; i++ {
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
			chal.Add(of.AccFr)
		}
	}

	ok, err = uinfo.verifyKey.VerifyProof(chal, prf)
	if err != nil {
		return err
	}

	if !ok {
		return xerrors.Errorf("wrong challenge proof at %d", scp.Epoch)
	}

	oinfo.prove = scp.Epoch + 1

	// save proof result
	key = store.NewKey(pb.MetaType_ST_SegProof, okey.userID, okey.proID, scp.Epoch)
	s.ds.Put(key, scp.Proof)

	key = store.NewKey(pb.MetaType_ST_SegProof, okey.userID, okey.proID)
	buf = make([]byte, 8)
	binary.BigEndian.PutUint64(buf, oinfo.prove)
	s.ds.Put(key, buf)

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

	if scp.Epoch != s.validateChalEpochInfo.Epoch {
		return xerrors.Errorf("wrong challenge epoch, expectd %d, got %d", s.validateChalEpochInfo.Epoch, scp.Epoch)
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

	buf := make([]byte, 8+len(s.chalEpochInfo.Seed.Bytes()))
	binary.BigEndian.PutUint64(buf[:8], okey.userID)
	copy(buf[8:], s.chalEpochInfo.Seed.Bytes())
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

	if ns.Nonce > 0 {
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

			chal.Add(sf.AccFr)
		} else {
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
			chal.Add(of.AccFr)
		}
	}

	if ns.Nonce > 1 {
		// todo: choose some from [0, ns.Nonce-1)
		for i := uint64(0); i < ns.Nonce-1; i++ {
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
			chal.Add(of.AccFr)
		}
	}

	ok, err = uinfo.verifyKey.VerifyProof(chal, prf)
	if err != nil {
		return err
	}

	if !ok {
		return xerrors.Errorf("wrong challenge proof at %d", scp.Epoch)
	}

	oinfo.prove = scp.Epoch + 1

	return nil
}
