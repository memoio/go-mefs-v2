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
	s.validateRInfo = make(map[uint64]*roleInfo)
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
		return s.validateRoot, xerrors.Errorf("apply block height is wrong: got %d, expected %d", blk.Height, s.height)
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
	s.Lock()
	defer s.Unlock()

	ri, ok := s.validateRInfo[msg.From]
	if !ok {
		ri = &roleInfo{
			Nonce: s.loadNonce(msg.From),
		}
		s.validateRInfo[msg.From] = ri
	}

	if msg.Nonce != ri.Nonce {
		return s.validateRoot, xerrors.Errorf("wrong nonce for: %d, expeted %d, got %d", msg.From, ri.Nonce, msg.Nonce)
	}

	ri.Nonce++
	s.newValidateRoot(msg.Params)

	switch msg.Method {
	case tx.CreateFs:
		err := s.canAddUser(msg)
		if err != nil {
			return s.validateRoot, err
		}
	case tx.CreateBucket:
		err := s.canAddBucket(msg)
		if err != nil {
			return s.validateRoot, err
		}
	case tx.DataPreOrder:
		err := s.canAddOrder(msg)
		if err != nil {
			return s.validateRoot, err
		}
	case tx.DataOrder:
		err := s.canAddSeq(msg)
		if err != nil {
			return s.validateRoot, err
		}
	case tx.UpdateEpoch:
		err := s.canUpdateChalEpoch(msg)
		if err != nil {
			return s.validateRoot, err
		}
	case tx.SegmentProof:
		err := s.canAddSegProof(msg)
		if err != nil {
			return s.validateRoot, err
		}
	default:
		return s.validateRoot, xerrors.Errorf("unsupported type: %d", msg.Method)
	}

	return s.validateRoot, nil
}
