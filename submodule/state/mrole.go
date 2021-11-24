package state

import (
	"encoding/binary"

	"github.com/gogo/protobuf/proto"

	pdpv2 "github.com/memoio/go-mefs-v2/lib/crypto/pdp/version2"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

func (s *StateMgr) getUser(userID uint64) (*segPerUser, error) {
	spu, ok := s.sInfo[userID]
	if ok {
		return spu, nil
	}

	spu = &segPerUser{
		buckets: make(map[uint64]*bucketManage),
	}

	key := store.NewKey(pb.MetaType_ST_RoleInfoKey, userID)

	data, err := s.ds.Get(key)
	if err != nil {
		return nil, err
	}

	pri := new(pb.RoleInfo)
	err = proto.Unmarshal(data, pri)
	if err != nil {
		return nil, err
	}

	vk := new(pdpv2.VerifyKey)
	err = vk.Deserialize(pri.BlsVerifyKey)
	if err != nil {
		return nil, err
	}

	spu.fsID = vk.Hash()
	spu.verifyKey = vk

	s.sInfo[userID] = spu

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

func (s *StateMgr) AddRole(pri *pb.RoleInfo) error {
	switch pri.Type {
	case pb.RoleInfo_Provider:
		s.Lock()
		has := false
		for _, uid := range s.pros {
			if uid == pri.ID {
				has = true
				break
			}
		}
		if !has {
			s.pros = append(s.pros, pri.ID)
		}
		s.Unlock()
	case pb.RoleInfo_User:
		vk := new(pdpv2.VerifyKey)
		err := vk.Deserialize(pri.BlsVerifyKey)
		if err != nil {
			return err
		}

		s.Lock()
		_, ok := s.sInfo[pri.ID]
		if ok {
			s.Unlock()
			return nil
		}

		s.users = append(s.pros, pri.ID)
		spu := &segPerUser{
			fsID: vk.Hash(),

			verifyKey: vk,
			buckets:   make(map[uint64]*bucketManage),
		}
		s.sInfo[pri.ID] = spu
		s.Unlock()
	default:
		return ErrRes
	}

	// save users
	key := store.NewKey(pb.MetaType_ST_RoleInfoKey, pri.ID)
	data, err := proto.Marshal(pri)
	if err != nil {
		return err
	}

	err = s.ds.Put(key, data)
	if err != nil {
		return err
	}

	return nil
}
