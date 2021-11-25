package state

import (
	"encoding/binary"

	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	pdpv2 "github.com/memoio/go-mefs-v2/lib/crypto/pdp/version2"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

func (s *StateMgr) loadUser(userID uint64) (*segPerUser, error) {
	key := store.NewKey(pb.MetaType_ST_PDPPublicKey, userID)
	data, err := s.ds.Get(key)
	if err != nil {
		return nil, err
	}

	pk := new(pdpv2.PublicKey)
	err = pk.Deserialize(data)
	if err != nil {
		return nil, err
	}

	spu := &segPerUser{
		buckets:   make(map[uint64]*bucketManage),
		fsID:      pk.VerifyKey().Hash(),
		verifyKey: pk.VerifyKey(),
	}

	// load bucket
	key = store.NewKey(pb.MetaType_ST_BucketOptKey, userID)
	data, err = s.ds.Get(key)
	if err != nil {
		return spu, nil
	}

	if len(data) >= 8 {
		spu.nextBucket = binary.BigEndian.Uint64(data)
	}

	return spu, nil
}

func (s *StateMgr) AddUser(userID uint64, pk pdpcommon.PublicKey) error {
	s.Lock()
	_, ok := s.sInfo[userID]
	if ok {
		s.Unlock()
		return nil
	}

	// verify vk
	spu := &segPerUser{
		fsID:      pk.VerifyKey().Hash(),
		verifyKey: pk.VerifyKey(),
		buckets:   make(map[uint64]*bucketManage),
	}
	s.sInfo[userID] = spu
	s.Unlock()

	// save users
	key := store.NewKey(pb.MetaType_ST_PDPPublicKey, userID)
	data := pk.Serialize()

	err := s.ds.Put(key, data)
	if err != nil {
		return err
	}

	return nil
}

func (s *StateMgr) CanAddUser(userID uint64, pk pdpcommon.PublicKey) error {
	s.Lock()
	_, ok := s.validateSInfo[userID]
	if ok {
		s.Unlock()
		return nil
	}

	// verify vk
	spu := &segPerUser{
		fsID: pk.VerifyKey().Hash(),

		verifyKey: pk.VerifyKey(),
		buckets:   make(map[uint64]*bucketManage),
	}
	s.validateSInfo[userID] = spu
	s.Unlock()

	return nil
}
