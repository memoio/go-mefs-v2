package state

import (
	"bytes"
	"encoding/binary"

	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/lib/crypto/pdp"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

// key: pb.MetaType_ST_PDPPublicKey/userID
func (s *StateMgr) loadUser(userID uint64) (*segPerUser, error) {
	var vk pdpcommon.VerifyKey
	key := store.NewKey(pb.MetaType_ST_PDPVerifyKey, userID)
	data, err := s.ds.Get(key)
	if err != nil {
		// should remove this
		key := store.NewKey(pb.MetaType_ST_PDPPublicKey, userID)
		data, err := s.ds.Get(key)
		if err != nil {
			return nil, err
		}
		pk, err := pdp.DeserializePublicKey(data)
		if err != nil {
			return nil, err
		}

		vk = pk.VerifyKey()

		key = store.NewKey(pb.MetaType_ST_PDPVerifyKey, userID)
		err = s.ds.Put(key, vk.Serialize())
		if err != nil {
			return nil, err
		}
	} else {
		vk, err = pdp.DeserializeVerifyKey(data)
		if err != nil {
			return nil, err
		}
	}

	spu := &segPerUser{
		userID:    userID,
		buckets:   make(map[uint64]*bucketManage),
		fsID:      vk.Hash(),
		verifyKey: vk,
	}

	// load bucket
	key = store.NewKey(pb.MetaType_ST_BucketOptKey, userID)
	data, err = s.ds.Get(key)
	if err == nil && len(data) >= 8 {
		spu.nextBucket = binary.BigEndian.Uint64(data)
	}
	return spu, nil
}

func (s *StateMgr) addUser(msg *tx.Message, tds store.TxnStore) error {
	pk, err := pdp.DeserializePublicKey(msg.Params)
	if err != nil {
		return err
	}

	_, ok := s.sInfo[msg.From]
	if ok {
		return nil
	}

	ri, ok := s.rInfo[msg.From]
	if !ok {
		ri = s.loadRole(msg.From)
		s.rInfo[msg.From] = ri
	}

	if ri.base.Type != pb.RoleInfo_User {
		return xerrors.Errorf("roletype is not user")
	}

	vk, err := pdp.DeserializeVerifyKey(ri.base.Extra)
	if err != nil {
		return err
	}

	if !bytes.Equal(vk.Hash(), pk.VerifyKey().Hash()) {
		return xerrors.Errorf("publickey does not match verifykey")
	}
	/*
		if !pdp.Validate(pk, vk) {
			return xerrors.Errorf("publickey does not match verifykey")
		}
	*/
	// verify vk
	spu := &segPerUser{
		userID:    msg.From,
		fsID:      pk.VerifyKey().Hash(),
		verifyKey: pk.VerifyKey(),
		buckets:   make(map[uint64]*bucketManage),
	}
	s.sInfo[msg.From] = spu

	// save user public key
	key := store.NewKey(pb.MetaType_ST_PDPPublicKey, msg.From)
	err = tds.Put(key, pk.Serialize())
	if err != nil {
		return err
	}

	// save user verify key
	key = store.NewKey(pb.MetaType_ST_PDPVerifyKey, msg.From)
	err = tds.Put(key, pk.VerifyKey().Serialize())
	if err != nil {
		return err
	}

	// save all users
	key = store.NewKey(pb.MetaType_ST_UsersKey)
	val, _ := tds.Get(key)
	buf := make([]byte, len(val)+8)
	copy(buf[:len(val)], val)
	binary.BigEndian.PutUint64(buf[len(val):len(val)+8], msg.From)
	err = tds.Put(key, buf)
	if err != nil {
		return err
	}

	if s.handleAddUser != nil {
		s.handleAddUser(msg.From)
	}

	return nil
}

func (s *StateMgr) canAddUser(msg *tx.Message) error {
	pk, err := pdp.DeserializePublicKey(msg.Params)
	if err != nil {
		return err
	}

	_, ok := s.validateSInfo[msg.From]
	if ok {
		return nil
	}

	ri, ok := s.validateRInfo[msg.From]
	if !ok {
		ri = s.loadRole(msg.From)
		s.validateRInfo[msg.From] = ri
	}

	if ri.base.Type != pb.RoleInfo_User {
		return xerrors.Errorf("roletype is not user")
	}

	vk, err := pdp.DeserializeVerifyKey(ri.base.Extra)
	if err != nil {
		return err
	}

	if !bytes.Equal(vk.Hash(), pk.VerifyKey().Hash()) {
		return xerrors.Errorf("publickey does not match verifykey")
	}

	/*
		if !pdp.Validate(pk, vk) {
			return xerrors.Errorf("publickey does not match verifykey")
		}
	*/

	// verify vk
	spu := &segPerUser{
		userID:    msg.From,
		fsID:      pk.VerifyKey().Hash(),
		verifyKey: pk.VerifyKey(),
		buckets:   make(map[uint64]*bucketManage),
	}
	s.validateSInfo[msg.From] = spu

	return nil
}
