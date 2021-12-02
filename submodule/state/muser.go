package state

import (
	"encoding/binary"

	"github.com/memoio/go-mefs-v2/lib/crypto/pdp"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

// key: pb.MetaType_ST_PDPPublicKey/userID
func (s *StateMgr) loadUser(userID uint64) (*segPerUser, error) {
	key := store.NewKey(pb.MetaType_ST_PDPPublicKey, userID)
	data, err := s.ds.Get(key)
	if err != nil {
		return nil, err
	}

	pk, err := pdp.DeserializePublicKey(data)
	if err != nil {
		return nil, err
	}

	spu := &segPerUser{
		userID:    userID,
		buckets:   make(map[uint64]*bucketManage),
		fsID:      pk.VerifyKey().Hash(),
		verifyKey: pk.VerifyKey(),
	}

	// load bucket
	key = store.NewKey(pb.MetaType_ST_BucketOptKey, userID)
	data, err = s.ds.Get(key)
	if err == nil && len(data) >= 8 {
		spu.nextBucket = binary.BigEndian.Uint64(data)
	}
	return spu, nil
}

func (s *StateMgr) addUser(msg *tx.Message) error {
	pk, err := pdp.DeserializePublicKey(msg.Params)
	if err != nil {
		return err
	}

	_, ok := s.sInfo[msg.From]
	if ok {
		return nil
	}

	// verify vk
	spu := &segPerUser{
		userID:    msg.From,
		fsID:      pk.VerifyKey().Hash(),
		verifyKey: pk.VerifyKey(),
		buckets:   make(map[uint64]*bucketManage),
	}
	s.sInfo[msg.From] = spu

	// save users
	key := store.NewKey(pb.MetaType_ST_PDPPublicKey, msg.From)
	data := pk.Serialize()
	err = s.ds.Put(key, data)
	if err != nil {
		return err
	}

	if s.hauf != nil {
		s.hauf(msg.From)
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
