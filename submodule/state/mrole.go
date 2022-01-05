package state

import (
	"encoding/binary"

	"github.com/gogo/protobuf/proto"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

func (s *StateMgr) loadVal(roleID uint64) *roleValue {
	rb := new(roleValue)
	key := store.NewKey(pb.MetaType_ST_RoleValueKey, roleID)
	data, err := s.ds.Get(key)
	if err != nil {
		return rb
	}

	rb.Deserialize(data)
	return rb
}

func (s *StateMgr) saveVal(roleID uint64, rv *roleValue, tds store.TxnStore) error {
	key := store.NewKey(pb.MetaType_ST_RoleValueKey, roleID)
	data, err := rv.Serialize()
	if err != nil {
		return err
	}
	return tds.Put(key, data)
}

func (s *StateMgr) loadRole(roleID uint64) *roleInfo {
	ri := &roleInfo{
		val: s.loadVal(roleID),
	}

	key := store.NewKey(pb.MetaType_ST_RoleBaseKey, roleID)
	data, err := s.ds.Get(key)
	if err == nil && len(data) > 0 {
		base := new(pb.RoleInfo)
		err := proto.Unmarshal(data, base)
		if err == nil {
			ri.base = base
		}
	}

	return ri
}

func (s *StateMgr) addRole(msg *tx.Message, tds store.TxnStore) error {
	pri := new(pb.RoleInfo)
	err := proto.Unmarshal(msg.Params, pri)
	if err != nil {
		return err
	}

	if pri.ID != msg.From {
		return xerrors.Errorf("wrong roleinfo for %d, expected: %d", pri.ID, msg.From)
	}

	// has?
	key := store.NewKey(pb.MetaType_ST_RoleBaseKey, pri.ID)
	ok, err := tds.Has(key)
	if err == nil && ok {
		return xerrors.Errorf("local already has role: %d", pri.ID)
	}

	ri, ok := s.rInfo[pri.ID]
	if ok {
		if ri.base != nil {
			return xerrors.Errorf("already has role: %d", pri.ID)
		}

		ri.base = pri
	} else {
		ri = &roleInfo{
			base: pri,
			val:  s.loadVal(pri.ID),
		}
		s.rInfo[pri.ID] = ri
	}

	// save
	err = tds.Put(key, msg.Params)
	if err != nil {
		return err
	}

	// save all roles
	switch pri.Type {
	case pb.RoleInfo_Keeper:
		s.keepers = append(s.keepers, msg.From)
		key = store.NewKey(pb.MetaType_ST_KeepersKey)
		val, _ := tds.Get(key)
		buf := make([]byte, len(val)+8)
		copy(buf[:len(val)], val)
		binary.BigEndian.PutUint64(buf[len(val):len(val)+8], msg.From)
		tds.Put(key, buf)
	case pb.RoleInfo_Provider:
		s.pros = append(s.pros, msg.From)
		key = store.NewKey(pb.MetaType_ST_ProsKey)
		val, _ := tds.Get(key)
		buf := make([]byte, len(val)+8)
		copy(buf[:len(val)], val)
		binary.BigEndian.PutUint64(buf[len(val):len(val)+8], msg.From)
		tds.Put(key, buf)
	case pb.RoleInfo_User:
		s.users = append(s.users, msg.From)
		key = store.NewKey(pb.MetaType_ST_UsersKey)
		val, _ := tds.Get(key)
		buf := make([]byte, len(val)+8)
		copy(buf[:len(val)], val)
		binary.BigEndian.PutUint64(buf[len(val):len(val)+8], msg.From)
		tds.Put(key, buf)
	}

	if s.handleAddRole != nil {
		s.handleAddRole(pri.ID, pri.Type)
	}

	return nil
}

func (s *StateMgr) canAddRole(msg *tx.Message) error {
	pri := new(pb.RoleInfo)
	err := proto.Unmarshal(msg.Params, pri)
	if err != nil {
		return err
	}

	if pri.ID != msg.From {
		return xerrors.Errorf("wrong roleinfo for %d, expected: %d", pri.ID, msg.From)
	}

	// has?
	key := store.NewKey(pb.MetaType_ST_RoleBaseKey, pri.ID)
	ok, err := s.ds.Has(key)
	if err == nil && ok {
		return xerrors.Errorf("local already has role: %d", pri.ID)
	}

	ri, ok := s.validateRInfo[pri.ID]
	if ok {
		if ri.base != nil {
			return xerrors.Errorf("already has role: %d", pri.ID)
		}

		ri.base = pri
		return nil
	}

	s.validateRInfo[pri.ID] = &roleInfo{
		base: pri,
		val:  s.loadVal(pri.ID),
	}

	return nil
}
