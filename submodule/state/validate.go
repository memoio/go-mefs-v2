package state

import (
	"github.com/zeebo/blake3"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
)

func (s *StateMgr) reset() {
	s.validateRoot = s.root
	s.validateHeight = s.height
	s.validateEpoch = s.epoch
	s.validateEpochInfo.Epoch = s.epochInfo.Epoch
	s.validateEpochInfo.Height = s.epochInfo.Height
	s.validateEpochInfo.Seed = s.epochInfo.Seed

	s.validateOInfo = make(map[orderKey]*orderInfo)
	s.validateSInfo = make(map[uint64]*segPerUser)
}

func (s *StateMgr) newValidateRoot(b []byte) {
	h := blake3.New()
	h.Write(s.validateRoot.Bytes())
	h.Write(b)
	res := h.Sum(nil)
	s.validateRoot = types.NewMsgID(res)
}

func (s *StateMgr) ValidateBlock(blk *tx.Block) (types.MsgID, error) {
	// time valid?
	if blk == nil {
		return s.validateRoot, nil
	}

	s.reset()

	if blk.Height != s.height {
		return s.validateRoot, xerrors.Errorf("apply block is wrong: got %d, expected %d, %w", blk.Height, s.height, ErrBlockHeight)
	}

	b, err := blk.RawHeader.Serialize()
	if err != nil {
		return s.validateRoot, err
	}

	s.newValidateRoot(b)

	return s.validateRoot, nil
}

func (s *StateMgr) ValidateMsg(msg *tx.Message) (types.MsgID, error) {
	if msg == nil {
		return s.validateRoot, nil
	}

	logger.Debug("validate message:", msg.From, msg.Nonce, msg.Method, s.validateRoot)
	switch msg.Method {
	case tx.CreateFs:
		return s.CanAddUser(msg)
	case tx.CreateBucket:
		return s.CanAddBucket(msg)
	case tx.DataPreOrder:
		return s.CanAddOrder(msg)
	case tx.DataOrder:
		return s.CanAddSeq(msg)
	case tx.UpdateEpoch:
		return s.CanUpdateEpoch(msg)
	case tx.SegmentProof:
		return s.CanAddSegProof(msg)
	default:
		return s.validateRoot, ErrRes
	}
}
