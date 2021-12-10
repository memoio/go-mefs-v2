package state

import (
	"github.com/zeebo/blake3"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
)

func (s *StateMgr) reset() {
	s.validateHeight = s.height
	s.validateSlot = s.slot
	s.validateRoot = s.root
	s.validateCeInfo.epoch = s.ceInfo.epoch
	s.validateCeInfo.current.Epoch = s.ceInfo.current.Epoch
	s.validateCeInfo.current.Slot = s.ceInfo.current.Slot
	s.validateCeInfo.current.Seed = s.ceInfo.current.Seed
	s.validateCeInfo.previous.Epoch = s.ceInfo.previous.Epoch
	s.validateCeInfo.previous.Slot = s.ceInfo.previous.Slot
	s.validateCeInfo.previous.Seed = s.ceInfo.previous.Seed

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
	s.Lock()
	defer s.Unlock()

	// time valid?
	if blk == nil {
		return s.validateRoot, nil
	}

	s.reset()

	if blk.Height != s.validateHeight {
		return s.validateRoot, xerrors.Errorf("apply block height is wrong: got %d, expected %d", blk.Height, s.height)
	}

	if blk.Slot <= s.validateSlot {
		return s.root, xerrors.Errorf("apply block epoch is wrong: got %d, expected larger than %d", blk.Slot, s.validateSlot)
	}

	b, err := blk.RawHeader.Serialize()
	if err != nil {
		return s.validateRoot, err
	}

	s.validateHeight++
	s.validateSlot = blk.Slot

	s.newValidateRoot(b)

	return s.validateRoot, nil
}

func (s *StateMgr) ValidateMsg(msg *tx.Message) (types.MsgID, error) {
	s.Lock()
	defer s.Unlock()

	if msg == nil {
		return s.validateRoot, nil
	}

	logger.Debug("validate message:", msg.From, msg.Nonce, msg.Method, s.validateRoot)

	ri, ok := s.validateRInfo[msg.From]
	if !ok {
		ri = s.loadRole(msg.From)
		s.validateRInfo[msg.From] = ri
	}

	if msg.Nonce != ri.val.Nonce {
		return s.validateRoot, xerrors.Errorf("wrong nonce for: %d, expeted %d, got %d", msg.From, ri.val.Nonce, msg.Nonce)
	}

	ri.val.Nonce++
	s.newValidateRoot(msg.Params)

	switch msg.Method {
	case tx.AddRole:
		err := s.canAddRole(msg)
		if err != nil {
			return s.validateRoot, err
		}
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
	case tx.SegmentFault:
		err := s.canRemoveSeg(msg)
		if err != nil {
			return s.validateRoot, err
		}
	case tx.UpdateChalEpoch:
		err := s.canUpdateChalEpoch(msg)
		if err != nil {
			return s.validateRoot, err
		}
	case tx.SegmentProof:
		err := s.canAddSegProof(msg)
		if err != nil {
			return s.validateRoot, err
		}
	case tx.PostIncome:
		err := s.canAddPay(msg)
		if err != nil {
			return s.validateRoot, err
		}
	default:
		return s.validateRoot, xerrors.Errorf("unsupported method: %d", msg.Method)
	}

	return s.validateRoot, nil
}
