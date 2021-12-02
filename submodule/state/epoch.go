package state

import (
	"bytes"
	"encoding/binary"

	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"golang.org/x/xerrors"
)

func (s *StateMgr) updateChalEpoch(msg *tx.Message) error {
	sep := new(tx.SignedEpochParams)
	err := sep.Deserialize(msg.Params)
	if err != nil {
		return err
	}

	if sep.Epoch != s.chalEpoch {
		return xerrors.Errorf("add chal epoch err: got %d, expected %d", sep.Epoch, s.chalEpoch)
	}

	if !bytes.Equal(sep.Prev.Bytes(), s.chalEpochInfo.Seed.Bytes()) {
		return xerrors.Errorf("add chal epoch seed err: got %s, expected %s", sep.Prev, s.chalEpochInfo.Seed)
	}

	if s.slot-s.chalEpochInfo.Slot < build.DefaultChalDuration {
		return xerrors.Errorf("add chal epoch err: duration is wrong")
	}

	s.chalEpoch++
	s.chalEpochInfo.Seed = types.NewMsgID(msg.Params)
	s.chalEpochInfo.Epoch = sep.Epoch
	s.chalEpochInfo.Slot = s.slot

	// store
	key := store.NewKey(pb.MetaType_ST_ChalEpochKey, s.chalEpochInfo.Epoch)
	data, err := s.chalEpochInfo.Serialize()
	if err != nil {
		return err
	}
	s.ds.Put(key, data)

	key = store.NewKey(pb.MetaType_ST_ChalEpochKey)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, s.chalEpoch)
	s.ds.Put(key, buf)

	return nil
}

func (s *StateMgr) canUpdateChalEpoch(msg *tx.Message) error {
	sep := new(tx.SignedEpochParams)
	err := sep.Deserialize(msg.Params)
	if err != nil {
		return err
	}

	if sep.Epoch != s.validateChalEpoch {
		return xerrors.Errorf("add chal epoch err: got %d, expected %d", sep.Epoch, s.validateChalEpoch)
	}

	if !bytes.Equal(sep.Prev.Bytes(), s.validateChalEpochInfo.Seed.Bytes()) {
		return xerrors.Errorf("add chal epoch seed err: got %s, expected %s", sep.Prev, s.validateChalEpochInfo.Seed)
	}

	if s.slot-s.validateChalEpochInfo.Slot < build.DefaultChalDuration {
		return xerrors.Errorf("add chal epoch err: duration is wrong")
	}

	s.validateChalEpoch++
	s.validateChalEpochInfo.Seed = types.NewMsgID(msg.Params)
	s.validateChalEpochInfo.Epoch = sep.Epoch
	s.validateChalEpochInfo.Slot = s.slot

	return nil
}
