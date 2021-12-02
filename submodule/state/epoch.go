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

	if sep.Epoch != s.epoch {
		return xerrors.Errorf("add chal epoch err: got %d, expected %d", sep.Epoch, s.epoch)
	}

	if !bytes.Equal(sep.Prev.Bytes(), s.epochInfo.Seed.Bytes()) {
		return xerrors.Errorf("add chal epoch seed err: got %s, expected %s", sep.Prev, s.epochInfo.Seed)
	}

	if s.height-s.epochInfo.Height < build.DefaultChalDuration {
		return xerrors.Errorf("add chal epoch err: duration is wrong")
	}

	s.epoch++
	s.epochInfo.Seed = types.NewMsgID(msg.Params)
	s.epochInfo.Epoch = sep.Epoch
	s.epochInfo.Height = s.height - 1

	// store
	key := store.NewKey(pb.MetaType_ST_ChalEpochKey, s.epochInfo.Epoch)
	data, err := s.epochInfo.Serialize()
	if err != nil {
		return err
	}
	s.ds.Put(key, data)

	key = store.NewKey(pb.MetaType_ST_ChalEpochKey)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, s.epoch)
	s.ds.Put(key, buf)

	return nil
}

func (s *StateMgr) canUpdateChalEpoch(msg *tx.Message) error {
	sep := new(tx.SignedEpochParams)
	err := sep.Deserialize(msg.Params)
	if err != nil {
		return err
	}

	if sep.Epoch != s.validateEpoch {
		return xerrors.Errorf("add chal epoch err: got %d, expected %d", sep.Epoch, s.validateEpoch)
	}

	if !bytes.Equal(sep.Prev.Bytes(), s.validateEpochInfo.Seed.Bytes()) {
		return xerrors.Errorf("add chal epoch seed err: got %s, expected %s", sep.Prev, s.validateEpochInfo.Seed)
	}

	if s.validateHeight-s.validateEpochInfo.Height < build.DefaultChalDuration {
		return xerrors.Errorf("add chal epoch err: duration is wrong")
	}

	s.validateEpoch++
	s.validateEpochInfo.Seed = types.NewMsgID(msg.Params)
	s.validateEpochInfo.Epoch = sep.Epoch
	s.validateEpochInfo.Height = s.validateHeight - 1

	return nil
}
