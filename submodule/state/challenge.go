package state

import (
	"encoding/binary"

	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12_381"
	"github.com/memoio/go-mefs-v2/lib/crypto/pdp"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/zeebo/blake3"
	"golang.org/x/xerrors"
)

func (s *StateMgr) AddSegProof(msg *tx.Message) (types.MsgID, error) {
	scp := new(tx.SegChalParams)
	err := scp.Deserialize(msg.Params)
	if err != nil {
		return s.root, err
	}
	prf, err := pdp.DeserializeProof(scp.Proof)
	if err != nil {
		return s.root, err
	}

	userID := msg.To
	proID := msg.From

	s.Lock()
	defer s.Unlock()

	if scp.Epoch != s.epochInfo.Epoch {
		return s.root, xerrors.Errorf("wrong challeng epoch, expectd %d, got %d", s.epochInfo.Epoch, scp.Epoch)
	}

	uinfo, ok := s.sInfo[userID]
	if !ok {
		uinfo, err = s.loadUser(userID)
		if err != nil {
			return s.root, err
		}
		s.sInfo[userID] = uinfo
	}

	lastChal, ok := uinfo.chalRes[proID]
	if !ok {
		lastChal = new(chalResult)
		key := store.NewKey(pb.MetaType_ST_SegProof, userID, proID)
		data, err := s.ds.Get(key)
		if err == nil && len(data) >= 8 {
			lastChal.Epoch = binary.BigEndian.Uint64(data)
		}

		uinfo.chalRes[proID] = lastChal
	}
	if lastChal.Epoch == scp.Epoch {
		return s.root, xerrors.Errorf("challeng proof submitted at %d", scp.Epoch)
	}

	buf := make([]byte, 8+len(s.epochInfo.Seed.Bytes()))
	binary.BigEndian.PutUint64(buf[:8], userID)
	copy(buf[8:], s.epochInfo.Seed.Bytes())
	bh := blake3.Sum256(buf)

	chal, _ := pdp.NewChallenge(uinfo.verifyKey, bh)

	for i := uint64(0); i < uinfo.nextBucket; i++ {
		key := store.NewKey(pb.MetaType_ST_SegMapKey, userID, i, proID, scp.Epoch)
		cm := new(chalManage)
		data, err := s.ds.Get(key)
		if err != nil {
			bm := s.getBucketManage(uinfo, i)
			cm = s.getChalManage(bm, userID, i, proID)
			data, err := cm.Serialize()
			if err != nil {
				return s.root, xerrors.Errorf("wrong challeng %d %w", scp.Epoch, err)
			}
			err = s.ds.Put(key, data)
			if err != nil {
				return s.root, xerrors.Errorf("wrong challeng %d %w", scp.Epoch, err)
			}
		} else {
			err := cm.Deserialize(data)
			if err != nil {
				return s.root, xerrors.Errorf("wrong challeng %d des err %w", scp.Epoch, err)
			}
		}

		chal.Add(bls.FrToBytes(&cm.accFr))
	}

	ok, err = uinfo.verifyKey.VerifyProof(chal, prf)
	if err != nil {
		return s.root, nil
	}

	if !ok {
		return s.root, xerrors.Errorf("wrong challeng proof at %d", scp.Epoch)
	}

	// save proof result
	key := store.NewKey(pb.MetaType_ST_SegProof, userID, proID, scp.Epoch)
	s.ds.Put(key, scp.Proof)

	key = store.NewKey(pb.MetaType_ST_SegProof, userID, proID)
	buf = make([]byte, 8)
	binary.BigEndian.PutUint64(buf, scp.Epoch)
	s.ds.Put(key, buf)

	s.newRoot(msg.Params)

	return s.root, nil
}

func (s *StateMgr) CanAddSegProof(msg *tx.Message) (types.MsgID, error) {
	scp := new(tx.SegChalParams)
	err := scp.Deserialize(msg.Params)
	if err != nil {
		return s.validateRoot, err
	}
	prf, err := pdp.DeserializeProof(scp.Proof)
	if err != nil {
		return s.validateRoot, err
	}

	userID := msg.To
	proID := msg.From

	s.Lock()
	defer s.Unlock()

	if scp.Epoch != s.epochInfo.Epoch {
		return s.validateRoot, xerrors.Errorf("wrong challeng epoch, expectd %d, got %d", s.epochInfo.Epoch, scp.Epoch)
	}

	uinfo, ok := s.validateSInfo[userID]
	if !ok {
		uinfo, err = s.loadUser(userID)
		if err != nil {
			return s.validateRoot, err
		}
		s.sInfo[userID] = uinfo
	}

	lastChal, ok := uinfo.chalRes[proID]
	if !ok {
		lastChal = new(chalResult)
		key := store.NewKey(pb.MetaType_ST_SegProof, userID, proID)
		data, err := s.ds.Get(key)
		if err == nil && len(data) >= 8 {
			lastChal.Epoch = binary.BigEndian.Uint64(data)
		}

		uinfo.chalRes[proID] = lastChal
	}
	if lastChal.Epoch == scp.Epoch {
		return s.root, xerrors.Errorf("challeng proof submitted at %d", scp.Epoch)
	}

	buf := make([]byte, 8+len(s.epochInfo.Seed.Bytes()))
	binary.BigEndian.PutUint64(buf[:8], userID)
	copy(buf[8:], s.epochInfo.Seed.Bytes())
	bh := blake3.Sum256(buf)

	chal, _ := pdp.NewChallenge(uinfo.verifyKey, bh)

	for i := uint64(0); i < uinfo.nextBucket; i++ {
		key := store.NewKey(pb.MetaType_ST_SegMapKey, userID, i, proID, scp.Epoch)
		cm := new(chalManage)
		data, err := s.ds.Get(key)
		if err != nil {
			bm := s.getBucketManage(uinfo, i)
			cm = s.getChalManage(bm, userID, i, proID)
			data, err := cm.Serialize()
			if err != nil {
				return s.validateRoot, xerrors.Errorf("wrong challeng %d %w", scp.Epoch, err)
			}
			err = s.ds.Put(key, data)
			if err != nil {
				return s.validateRoot, xerrors.Errorf("wrong challeng %d %w", scp.Epoch, err)
			}
		} else {
			err := cm.Deserialize(data)
			if err != nil {
				return s.validateRoot, xerrors.Errorf("wrong challeng %d des err %w", scp.Epoch, err)
			}
		}

		chal.Add(bls.FrToBytes(&cm.accFr))
	}

	ok, err = uinfo.verifyKey.VerifyProof(chal, prf)
	if err != nil {
		return s.validateRoot, nil
	}

	if !ok {
		return s.validateRoot, xerrors.Errorf("wrong challeng proof at %d", scp.Epoch)
	}

	lastChal.Epoch = scp.Epoch

	s.newValidateRoot(msg.Params)

	return s.validateRoot, nil
}
