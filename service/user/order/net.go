package order

import (
	"github.com/zeebo/blake3"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
)

func (m *OrderMgr) connect(proID uint64) error {
	pi, err := m.GetNetInfo(m.ctx, proID)
	if err == nil {
		m.ns.AddNode(proID, pi.ID)
		m.ns.Host().Connect(m.ctx, pi)
	}

	// test remote service is ready or not
	_, err = m.ns.SendMetaRequest(m.ctx, proID, pb.NetMessage_AskPrice, nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (m *OrderMgr) update(proID uint64) {
	err := m.connect(proID)
	if err != nil {
		return
	}

	m.updateChan <- proID
}

func (m *OrderMgr) getQuotation(proID uint64) error {
	logger.Debug("get new quotation from: ", proID)
	resp, err := m.ns.SendMetaRequest(m.ctx, proID, pb.NetMessage_AskPrice, nil, nil)
	if err != nil {
		return err
	}

	if resp.GetHeader().GetFrom() != proID {
		logger.Debug("fail get new quotation from: ", proID)
		return xerrors.Errorf("wrong quotation from expected %d, got %d", proID, resp.GetHeader().GetFrom())
	}

	quo := new(types.Quotation)
	err = quo.Deserialize(resp.GetData().GetMsgInfo())
	if err != nil {
		logger.Debug("fail get new quotation from: ", proID, err)
		return err
	}

	sig := new(types.Signature)
	err = sig.Deserialize(resp.GetData().GetSign())
	if err != nil {
		logger.Debug("fail get new quotation from: ", proID, err)
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
	sig, err := m.RoleSign(m.ctx, m.localID, msg[:], types.SigSecp256k1)
	if err != nil {
		return err
	}

	sigByte, err := sig.Serialize()
	if err != nil {
		return err
	}

	resp, err := m.ns.SendMetaRequest(m.ctx, proID, pb.NetMessage_CreateOrder, data, sigByte)
	if err != nil {
		return err
	}

	if resp.GetHeader().GetType() == pb.NetMessage_Err {
		logger.Debug("fail get new order ack from: ", proID)
		return ErrNotFound
	}

	ob := new(types.SignedOrder)
	err = ob.Deserialize(resp.GetData().GetMsgInfo())
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
	sig, err := m.RoleSign(m.ctx, m.localID, msg[:], types.SigSecp256k1)
	if err != nil {
		return err
	}

	sigByte, err := sig.Serialize()
	if err != nil {
		return err
	}

	resp, err := m.ns.SendMetaRequest(m.ctx, proID, pb.NetMessage_CreateSeq, data, sigByte)
	if err != nil {
		return err
	}

	if resp.GetHeader().GetType() == pb.NetMessage_Err {
		logger.Debug("fail get new seq ack from: ", proID)
		return ErrNotFound
	}

	os := new(types.SignedOrderSeq)
	err = os.Deserialize(resp.GetData().GetMsgInfo())
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
	sig, err := m.RoleSign(m.ctx, m.localID, msg[:], types.SigSecp256k1)
	if err != nil {
		return err
	}

	sigByte, err := sig.Serialize()
	if err != nil {
		return err
	}

	resp, err := m.ns.SendMetaRequest(m.ctx, proID, pb.NetMessage_FinishSeq, data, sigByte)
	if err != nil {
		return err
	}

	if resp.GetHeader().GetType() == pb.NetMessage_Err {
		logger.Debug("fail get finish seq ack from: ", proID)
		return ErrNotFound
	}

	os := new(types.SignedOrderSeq)
	err = os.Deserialize(resp.GetData().GetMsgInfo())
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
