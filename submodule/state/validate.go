package state

import (
	"time"

	"github.com/zeebo/blake3"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
)

func (s *StateMgr) reset() {
	s.validateVersion = s.version
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

// only validate txs;
// sign is valid outside; only valid sign len here
func (s *StateMgr) ValidateBlock(blk *tx.SignedBlock) (types.MsgID, error) {
	s.lk.Lock()
	defer s.lk.Unlock()

	s.reset()

	// time valid?
	if blk == nil {
		return s.validateRoot, nil
	}

	if !blk.PrevID.Equal(s.blkID) {
		return s.validateRoot, xerrors.Errorf("validate block prvID is wrong: got %s, expected %s", blk.PrevID, s.blkID)
	}

	if blk.Height != s.validateHeight {
		return s.validateRoot, xerrors.Errorf("validate block height is wrong: got %d, expected %d", blk.Height, s.height)
	}

	if blk.Slot <= s.validateSlot {
		return s.validateRoot, xerrors.Errorf("validate block epoch is wrong: got %d, expected larger than %d", blk.Slot, s.validateSlot)
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
	s.lk.Lock()
	defer s.lk.Unlock()

	if msg == nil {
		return s.validateRoot, nil
	}

	nt := time.Now()

	ri, ok := s.validateRInfo[msg.From]
	if !ok {
		// load from
		nri, ok := s.rInfo[msg.From]
		if ok {
			ri = &roleInfo{
				base: nri.base,
				val: &roleValue{
					Nonce: nri.val.Nonce,
					//Value: new(big.Int).Set(nri.val.Value),
				},
			}
		} else {
			ri = s.loadRole(msg.From)
		}
		s.validateRInfo[msg.From] = ri
	}

	if msg.Nonce != ri.val.Nonce {
		return s.validateRoot, xerrors.Errorf("validate nonce is wrong for: %d, expeted %d, got %d", msg.From, ri.val.Nonce, msg.Nonce)
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
	case tx.PreDataOrder:
		err := s.canCreateOrder(msg)
		if err != nil {
			return s.validateRoot, err
		}
	case tx.AddDataOrder:
		err := s.canAddSeq(msg)
		if err != nil {
			return s.validateRoot, err
		}
	case tx.CommitDataOrder:
		err := s.canCommitOrder(msg)
		if err != nil {
			return s.validateRoot, err
		}
	case tx.SubDataOrder:
		err := s.canSubOrder(msg)
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
	case tx.ConfirmPostIncome:
		err := s.canAddPay(msg)
		if err != nil {
			return s.validateRoot, err
		}
	case tx.UpdateNet:
		err := s.canUpdateNetAddr(msg)
		if err != nil {
			return s.validateRoot, err
		}
	default:
		return s.validateRoot, xerrors.Errorf("unsupported method: %d", msg.Method)
	}

	logger.Debug("validate message: ", s.validateHeight, msg.From, msg.Nonce, msg.Method, s.validateRoot, time.Since(nt))

	return s.validateRoot, nil
}
