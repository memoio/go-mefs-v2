package order

import (
	"time"

	"github.com/zeebo/blake3"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
)

func (m *OrderMgr) connect(proID uint64) error {
	// test remote service is ready or not
	resp, err := m.ns.SendMetaRequest(m.ctx, proID, pb.NetMessage_AskPrice, nil, nil)
	if err == nil {
		if resp.GetHeader().GetFrom() == proID && resp.GetHeader().GetType() != pb.NetMessage_Err {
			return nil
		}
	}

	// otherwise get addr from declared address
	pi, err := m.StateGetNetInfo(m.ctx, proID)
	if err == nil {
		// todo: fix this
		err := m.ns.Host().Connect(m.ctx, pi)
		if err == nil {
			m.ns.Host().Peerstore().SetAddrs(pi.ID, pi.Addrs, time.Duration(24*time.Hour))
			m.ns.AddNode(proID, pi)
		}
		logger.Debugf("connect pro declared: %s %s", pi, err)

		if err != nil {
			return err
		}
	}

	// test remote service is ready or not
	resp, err = m.ns.SendMetaRequest(m.ctx, proID, pb.NetMessage_AskPrice, nil, nil)
	if err != nil {
		return err
	}

	if resp.GetHeader().GetFrom() != proID {
		return xerrors.Errorf("wrong connect from expected %d, got %d", proID, resp.GetHeader().GetFrom())
	}

	if resp.GetHeader().GetType() == pb.NetMessage_Err {
		return xerrors.Errorf("connect %d fail %s", proID, resp.GetData().MsgInfo)
	}

	return nil
}

func (m *OrderMgr) update(proID uint64) {
	err := m.connect(proID)
	if err != nil {
		logger.Debug("fail connect: ", proID, err)
		return
	}

	m.updateChan <- proID
}

func (m *OrderMgr) getOrderRemote(proID uint64) (*types.SignedOrder, error) {
	logger.Debug("get orders from: ", proID)
	// test remote service is ready or not
	resp, err := m.ns.SendMetaRequest(m.ctx, proID, pb.NetMessage_GetOrder, nil, nil)
	if err != nil {
		return nil, err
	}

	if resp.GetHeader().GetFrom() != proID {
		return nil, xerrors.Errorf("wrong get order from expected %d, got %d", proID, resp.GetHeader().GetFrom())
	}

	if resp.GetHeader().GetType() == pb.NetMessage_Err {
		return nil, xerrors.Errorf("get order from %d fail %s", proID, resp.GetData().MsgInfo)
	}

	ob := new(types.SignedOrder)
	err = ob.Deserialize(resp.GetData().GetMsgInfo())
	if err != nil {
		logger.Debug("fail get order from: ", proID, err)
		return nil, err
	}

	// verify order

	return ob, nil
}

func (m *OrderMgr) getSeqRemote(proID uint64) (*types.SignedOrderSeq, error) {
	logger.Debug("get seq from: ", proID)
	// test remote service is ready or not
	resp, err := m.ns.SendMetaRequest(m.ctx, proID, pb.NetMessage_GetOrder, nil, nil)
	if err != nil {
		return nil, err
	}

	if resp.GetHeader().GetFrom() != proID {
		return nil, xerrors.Errorf("wrong get order seq from expected %d, got %d", proID, resp.GetHeader().GetFrom())
	}

	if resp.GetHeader().GetType() == pb.NetMessage_Err {
		return nil, xerrors.Errorf("get order seq from %d fail %s", proID, resp.GetData().MsgInfo)
	}

	os := new(types.SignedOrderSeq)
	err = os.Deserialize(resp.GetData().GetMsgInfo())
	if err != nil {
		logger.Debug("fail get order seq: ", proID, err)
		return nil, err
	}

	// verify order

	return os, nil
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

	if resp.GetHeader().GetType() == pb.NetMessage_Err {
		logger.Debug("fail get new quotation from: ", proID, string(resp.GetData().MsgInfo))
		return xerrors.Errorf("get new quotation from %d fail %s", proID, resp.GetData().MsgInfo)
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
		logger.Debugf("fail get new order ack from %d %s ", proID, string(resp.GetData().MsgInfo))
		return xerrors.Errorf("get new order ack from %d fail %s", proID, resp.GetData().MsgInfo)
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
		logger.Debug("fail get new seq ack from: ", proID, string(resp.GetData().MsgInfo))
		return xerrors.Errorf("get new seq ack from %d fail %s", proID, resp.GetData().MsgInfo)
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
		logger.Debug("fail get finish seq ack from: ", proID, string(resp.GetData().MsgInfo))
		return xerrors.Errorf("get finish seq ack from %d fail %s", proID, resp.GetData().MsgInfo)
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
