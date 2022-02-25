package state

import (
	"encoding/binary"

	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

func (s *StateMgr) updateChalEpoch(msg *tx.Message, tds store.TxnStore) error {
	sep := new(tx.SignedEpochParams)
	err := sep.Deserialize(msg.Params)
	if err != nil {
		return err
	}

	if sep.Epoch != s.ceInfo.epoch {
		return xerrors.Errorf("add chal epoch err: got %d, expected %d", sep.Epoch, s.ceInfo.epoch)
	}

	if !sep.Prev.Equal(s.ceInfo.current.Seed) {
		return xerrors.Errorf("add chal epoch seed err: got %s, expected %s", sep.Prev, s.ceInfo.current.Seed)
	}

	curChalEpoch, ok := build.ChalDurMap[s.version]
	if !ok {
		return xerrors.Errorf("add chal epoch err: duration is wrong")
	}

	if s.slot-s.ceInfo.current.Slot < curChalEpoch {
		return xerrors.Errorf("add chal epoch err: duration is wrong, expect at least %d", curChalEpoch)
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
	err = tds.Put(key, data)
	if err != nil {
		return err
	}

	key = store.NewKey(pb.MetaType_ST_ChalEpochKey)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, s.ceInfo.epoch)
	err = tds.Put(key, buf)
	if err != nil {
		return err
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

	if !sep.Prev.Equal(s.validateCeInfo.current.Seed) {
		return xerrors.Errorf("add chal epoch seed err: got %s, expected %s", sep.Prev, s.validateCeInfo.current.Seed)
	}

	curChalEpoch, ok := build.ChalDurMap[s.validateVersion]
	if !ok {
		return xerrors.Errorf("add chal epoch err: duration is wrong")
	}

	if s.validateSlot-s.validateCeInfo.current.Slot < curChalEpoch {
		return xerrors.Errorf("add chal epoch err: duration is wrong, expect at least %d", curChalEpoch)
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
