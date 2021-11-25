package state

import (
	pdpv2 "github.com/memoio/go-mefs-v2/lib/crypto/pdp/version2"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
)

func (s *StateMgr) reset() {
	s.validateOInfo = make(map[orderKey]*orderInfo)
	s.validateSInfo = make(map[uint64]*segPerUser)
}

func (s *StateMgr) ValidateMsg(msg *tx.Message) error {
	if msg == nil {
		s.reset()
		return nil
	}
	logger.Debug("validate message:", msg.From, msg.Nonce, msg.Method)
	switch msg.Method {
	case tx.CreateFs:
		pk := new(pdpv2.PublicKey)
		err := pk.Deserialize(msg.Params)
		if err != nil {
			return err
		}
		return s.CanAddUser(msg.From, pk)
	case tx.CreateBucket:
		tbp := new(tx.BucketParams)
		err := tbp.Deserialize(msg.Params)
		if err != nil {
			return err
		}
		return s.CanAddBucket(msg.From, tbp.BucketID, &tbp.BucketOption)
	case tx.DataPreOrder:
		so := new(types.SignedOrder)
		err := so.Deserialize(msg.Params)
		if err != nil {
			return err
		}
		return s.CanAddOrder(so)
	case tx.DataOrder:
		so := new(types.SignedOrderSeq)
		err := so.Deserialize(msg.Params)
		if err != nil {
			return err
		}
		return s.CanAddSeq(so)
	default:
		return ErrRes
	}
}
