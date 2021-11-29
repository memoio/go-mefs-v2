package state

import (
	"github.com/zeebo/blake3"

	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
)

func (s *StateMgr) reset() {
	s.validateRoot = s.root

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

func (s *StateMgr) ValidateMsg(msg *tx.Message) (types.MsgID, error) {
	if msg == nil {
		s.reset()
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
	default:
		return s.validateRoot, ErrRes
	}
}
