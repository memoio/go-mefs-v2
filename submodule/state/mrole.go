package state

import (
	"github.com/gogo/protobuf/proto"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"golang.org/x/xerrors"
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

func (s *StateMgr) saveVal(roleID uint64, rv *roleValue) error {
	key := store.NewKey(pb.MetaType_ST_RoleValueKey, roleID)
	data, err := rv.Serialize()
	if err != nil {
		return err
	}
	return s.ds.Put(key, data)
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

func (s *StateMgr) addRole(msg *tx.Message) error {
	pri := new(pb.RoleInfo)
	err := proto.Unmarshal(msg.Params, pri)
	if err != nil {
		return err
	}

	// has?
	key := store.NewKey(pb.MetaType_ST_RoleBaseKey, msg.From)
	ok, err := s.ds.Has(key)
	if err == nil && ok {
		return xerrors.Errorf("local already has role: %d", msg.From)
	}

	ri, ok := s.rInfo[msg.From]
	if ok {
		if ri.base != nil {
			return xerrors.Errorf("already has role: %d", msg.From)
		}

		ri.base = pri
	} else {
		ri = &roleInfo{
			base: pri,
			val:  s.loadVal(msg.From),
		}
		s.rInfo[msg.From] = ri
	}

	// save
	err = s.ds.Put(key, msg.Params)
	if err != nil {
		return err
	}

	return nil
}

func (s *StateMgr) canAddRole(msg *tx.Message) error {
	pri := new(pb.RoleInfo)
	err := proto.Unmarshal(msg.Params, pri)
	if err != nil {
		return err
	}

	// has?
	key := store.NewKey(pb.MetaType_ST_RoleBaseKey, msg.From)
	ok, err := s.ds.Has(key)
	if err == nil && ok {
		return xerrors.Errorf("local already has role: %d", msg.From)
	}

	ri, ok := s.validateRInfo[msg.From]
	if ok {
		if ri.base != nil {
			return xerrors.Errorf("already has role: %d", msg.From)
		}

		ri.base = pri
		return nil
	}

	s.validateRInfo[msg.From] = &roleInfo{
		base: pri,
		val:  s.loadVal(msg.From),
	}

	return nil
}
