package order

import (
	"github.com/fxamacker/cbor/v2"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/zeebo/blake3"
)

func (m *OrderMgr) connect(proID uint64) error {
	_, err := m.SendMetaRequest(m.ctx, proID, pb.NetMessage_SayHello, nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (m *OrderMgr) getQuotation(proID uint64) error {
	logger.Debug("get new quotation from: ", proID)
	resp, err := m.SendMetaRequest(m.ctx, proID, pb.NetMessage_AskPrice, nil, nil)
	if err != nil {
		return err
	}

	if resp.GetHeader().GetFrom() != proID {
		logger.Debug("fail get new quotation from: ", proID)
		return ErrState
	}

	quo := new(types.Quotation)
	err = cbor.Unmarshal(resp.GetData().GetMsgInfo(), quo)
	if err != nil {
		logger.Debug("fail get new quotation from: ", proID)
		return err
	}

	sig := new(types.Signature)
	err = sig.Deserialize(resp.GetData().GetSign())
	if err != nil {
		logger.Debug("fail get new quotation from: ", proID)
		return err
	}

	// verify
	msg := blake3.Sum256(resp.GetData().GetMsgInfo())
	ok, _ := m.RoleVerify(m.ctx, proID, msg[:], *sig)
	if ok {
		m.quoChan <- quo
	}

	return nil
}

func (m *OrderMgr) getNewOrderAck(proID uint64, data []byte) error {
	logger.Debug("get new order ack from: ", proID)
	msg := blake3.Sum256(data)
	sig, err := m.RoleSign(m.ctx, msg[:], types.SigSecp256k1)
	if err != nil {
		return err
	}

	sigByte, err := sig.Serialize()
	if err != nil {
		return err
	}

	resp, err := m.SendMetaRequest(m.ctx, proID, pb.NetMessage_CreateOrder, data, sigByte)
	if err != nil {
		return err
	}

	if resp.GetHeader().GetType() == pb.NetMessage_Err {
		logger.Debug("fail get new order ack from: ", proID)
		return ErrNotFound
	}

	ob := new(types.OrderBase)
	err = cbor.Unmarshal(resp.GetData().GetMsgInfo(), ob)
	if err != nil {
		logger.Debug("fail get new order ack from: ", proID, err)
		return err
	}

	psig := new(types.Signature)
	err = psig.Deserialize(resp.GetData().GetSign())
	if err != nil {
		logger.Debug("fail get new order ack from: ", proID, err)
		return err
	}

	pmsg := blake3.Sum256(resp.GetData().GetMsgInfo())
	ok, _ := m.RoleVerify(m.ctx, proID, pmsg[:], *psig)
	if ok {
		m.orderChan <- ob
	}

	return nil
}

func (m *OrderMgr) getNewSeqAck(proID uint64, data []byte) error {
	logger.Debug("get new seq ack from: ", proID)
	msg := blake3.Sum256(data)
	sig, err := m.RoleSign(m.ctx, msg[:], types.SigSecp256k1)
	if err != nil {
		return err
	}

	sigByte, err := sig.Serialize()
	if err != nil {
		return err
	}

	resp, err := m.SendMetaRequest(m.ctx, proID, pb.NetMessage_CreateSeq, data, sigByte)
	if err != nil {
		return err
	}

	if resp.GetHeader().GetType() == pb.NetMessage_Err {
		logger.Debug("fail get new seq ack from: ", proID)
		return ErrNotFound
	}

	os := new(types.OrderSeq)
	err = cbor.Unmarshal(resp.GetData().GetMsgInfo(), os)
	if err != nil {
		logger.Debug("fail get new seq ack from: ", proID, err)
		return err
	}

	psig := new(types.Signature)
	err = psig.Deserialize(resp.GetData().GetSign())
	if err != nil {
		logger.Debug("fail get new seq ack from: ", proID, err)
		return err
	}

	pmsg := blake3.Sum256(resp.GetData().GetMsgInfo())
	ok, _ := m.RoleVerify(m.ctx, proID, pmsg[:], *psig)
	if ok {
		osp := &orderSeqPro{
			proID: proID,
			os:    os,
		}
		m.seqNewChan <- osp
	}

	return nil
}

func (m *OrderMgr) getSeqFinishAck(proID uint64, data []byte) error {
	logger.Debug("get finish seq ack from: ", proID)
	msg := blake3.Sum256(data)
	sig, err := m.RoleSign(m.ctx, msg[:], types.SigSecp256k1)
	if err != nil {
		return err
	}

	sigByte, err := sig.Serialize()
	if err != nil {
		return err
	}

	resp, err := m.SendMetaRequest(m.ctx, proID, pb.NetMessage_FinishSeq, data, sigByte)
	if err != nil {
		return err
	}

	if resp.GetHeader().GetType() == pb.NetMessage_Err {
		logger.Debug("fail get finish seq ack from: ", proID)
		return ErrNotFound
	}

	os := new(types.OrderSeq)
	err = cbor.Unmarshal(resp.GetData().GetMsgInfo(), os)
	if err != nil {
		logger.Debug("fail get finish seq ack from: ", proID, err)
		return err
	}

	psig := new(types.Signature)
	err = psig.Deserialize(resp.GetData().GetSign())
	if err != nil {
		logger.Debug("fail get finish seq ack from: ", proID, err)
		return err
	}

	pmsg := blake3.Sum256(resp.GetData().GetMsgInfo())
	ok, _ := m.RoleVerify(m.ctx, proID, pmsg[:], *psig)
	if ok {
		osp := &orderSeqPro{
			proID: proID,
			os:    os,
		}
		m.seqFinishChan <- osp
	}

	return nil
}
