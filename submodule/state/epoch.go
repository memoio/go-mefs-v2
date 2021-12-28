package state

import (
	"bytes"
	"encoding/binary"

	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

func (s *StateMgr) updateChalEpoch(msg *tx.Message) error {
	sep := new(tx.SignedEpochParams)
	err := sep.Deserialize(msg.Params)
	if err != nil {
		return err
	}

	if sep.Epoch != s.ceInfo.epoch {
		return xerrors.Errorf("add chal epoch err: got %d, expected %d", sep.Epoch, s.ceInfo.epoch)
	}

	if !bytes.Equal(sep.Prev.Bytes(), s.ceInfo.current.Seed.Bytes()) {
		return xerrors.Errorf("add chal epoch seed err: got %s, expected %s", sep.Prev, s.ceInfo.current.Seed)
	}

	if s.slot-s.ceInfo.current.Slot < build.DefaultChalDuration {
		return xerrors.Errorf("add chal epoch err: duration is wrong")
	}

	s.ceInfo.epoch++
	s.ceInfo.previous.Epoch = s.ceInfo.current.Epoch
	s.ceInfo.previous.Slot = s.ceInfo.current.Slot
	s.ceInfo.previous.Seed = s.ceInfo.current.Seed

	s.ceInfo.current.Epoch = sep.Epoch
	s.ceInfo.current.Slot = s.slot
	s.ceInfo.current.Seed = types.NewMsgID(msg.Params)

	// store
	key := store.NewKey(pb.MetaType_ST_ChalEpochKey, s.ceInfo.current.Epoch)
	data, err := s.ceInfo.current.Serialize()
	if err != nil {
		return err
	}
	s.ds.Put(key, data)

	key = store.NewKey(pb.MetaType_ST_ChalEpochKey)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, s.ceInfo.epoch)
	s.ds.Put(key, buf)

	// store for pro pay
	for _, pid := range s.pros {
		key := store.NewKey(pb.MetaType_ST_SegPayKey, pid)
		val, err := s.ds.Get(key)
		if err == nil {
			key := store.NewKey(pb.MetaType_ST_SegPayKey, 0, pid, s.ceInfo.previous.Epoch)
			s.ds.Put(key, val)
		}
	}

	return nil
}

func (s *StateMgr) canUpdateChalEpoch(msg *tx.Message) error {
	sep := new(tx.SignedEpochParams)
	err := sep.Deserialize(msg.Params)
	if err != nil {
		return err
	}

	if sep.Epoch != s.validateCeInfo.epoch {
		return xerrors.Errorf("add chal epoch err: got %d, expected %d", sep.Epoch, s.validateCeInfo.epoch)
	}

	if !bytes.Equal(sep.Prev.Bytes(), s.validateCeInfo.current.Seed.Bytes()) {
		return xerrors.Errorf("add chal epoch seed err: got %s, expected %s", sep.Prev, s.validateCeInfo.current.Seed)
	}

	if s.validateSlot-s.validateCeInfo.current.Slot < build.DefaultChalDuration {
		return xerrors.Errorf("add chal epoch err: duration is wrong")
	}

	s.validateCeInfo.epoch++
	s.validateCeInfo.previous.Epoch = s.validateCeInfo.current.Epoch
	s.validateCeInfo.previous.Slot = s.validateCeInfo.current.Slot
	s.validateCeInfo.previous.Seed = s.validateCeInfo.current.Seed

	s.validateCeInfo.current.Epoch = sep.Epoch
	s.validateCeInfo.current.Slot = s.slot
	s.validateCeInfo.current.Seed = types.NewMsgID(msg.Params)

	return nil
}
