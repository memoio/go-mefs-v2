package state

import (
	"bytes"

	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
)

func (s *StateMgr) UpdateEpoch(msg *tx.Message) (types.MsgID, error) {
	sep := new(tx.SignedEpochParams)
	err := sep.Deserialize(msg.Params)
	if err != nil {
		return s.root, err
	}

	s.Lock()
	defer s.Unlock()

	if sep.Epoch != s.epoch {
		return s.root, ErrEpoch
	}

	if !bytes.Equal(sep.Prev.Bytes(), s.epochInfo.Seed.Bytes()) {
		return s.root, ErrEpoch
	}

	if s.height-s.epochInfo.Height < build.DefaultChalDuration {
		return s.validateRoot, ErrEpoch
	}

	s.epochInfo.Seed = types.NewMsgID(msg.Params)

	s.newRoot(msg.Params)

	return s.root, nil
}

func (s *StateMgr) CanUpdateEpoch(msg *tx.Message) (types.MsgID, error) {
	sep := new(tx.SignedEpochParams)
	err := sep.Deserialize(msg.Params)
	if err != nil {
		return s.validateRoot, err
	}

	s.Lock()
	defer s.Unlock()

	if sep.Epoch != s.validateEpoch {
		return s.validateRoot, ErrEpoch
	}

	if !bytes.Equal(sep.Prev.Bytes(), s.validateEpochInfo.Seed.Bytes()) {
		return s.validateRoot, ErrEpoch
	}

	if s.validateHeight-s.validateEpochInfo.Height < build.DefaultChalDuration {
		return s.validateRoot, ErrEpoch
	}

	s.validateEpoch++
	s.validateEpochInfo.Seed = types.NewMsgID(msg.Params)
	s.validateEpochInfo.Epoch = sep.Epoch
	s.validateEpochInfo.Height = s.validateHeight - 1

	s.newValidateRoot(msg.Params)

	return s.root, nil
}
