package order

import (
	"github.com/fxamacker/cbor/v2"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/zeebo/blake3"
)

func (m *OrderMgr) connect(proID uint64) {
	_, err := m.SendMetaRequest(m.ctx, proID, pb.NetMessage_SayHello, nil, nil)
	if err != nil {
		return
	}
}

func (m *OrderMgr) getQuotation(proID uint64) error {
	resp, err := m.SendMetaRequest(m.ctx, proID, pb.NetMessage_AskPrice, nil, nil)
	if err != nil {
		return err
	}

	if resp.GetHeader().GetFrom() != proID {
		return ErrState
	}

	quo := new(types.Quotation)
	err = cbor.Unmarshal(resp.GetData().GetMsgInfo(), quo)
	if err != nil {
		return err
	}

	sig := new(types.Signature)
	err = sig.Deserialize(resp.GetData().GetSign())
	if err != nil {
		return err
	}

	// verify

	msg := blake3.Sum256(resp.GetData().GetMsgInfo())
	ok := m.RoleVerify(proID, msg[:], *sig)
	if ok {
		m.quoChan <- quo
	}

	return nil
}

func (m *OrderMgr) getNewOrderAck(proID uint64, data []byte) error {
	msg := blake3.Sum256(data)
	sig, err := m.RoleSign(msg[:], types.SigSecp256k1)
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
		return ErrNotFound
	}

	ob := new(types.OrderBase)
	err = cbor.Unmarshal(resp.GetData().GetMsgInfo(), ob)
	if err != nil {
		return err
	}

	psig := new(types.Signature)
	err = psig.Deserialize(resp.GetData().GetSign())
	if err != nil {
		return err
	}

	pmsg := blake3.Sum256(resp.GetData().GetMsgInfo())
	ok := m.RoleVerify(proID, pmsg[:], *psig)
	if ok {
		m.orderChan <- ob
	}

	return nil
}

func (m *OrderMgr) getNewSeqAck(proID uint64, data []byte) error {
	msg := blake3.Sum256(data)
	sig, err := m.RoleSign(msg[:], types.SigSecp256k1)
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
		return ErrNotFound
	}

	os := new(types.OrderSeq)
	err = cbor.Unmarshal(resp.GetData().GetMsgInfo(), os)
	if err != nil {
		return err
	}

	psig := new(types.Signature)
	err = psig.Deserialize(resp.GetData().GetSign())
	if err != nil {
		return err
	}

	pmsg := blake3.Sum256(resp.GetData().GetMsgInfo())
	ok := m.RoleVerify(proID, pmsg[:], *psig)
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
	msg := blake3.Sum256(data)
	sig, err := m.RoleSign(msg[:], types.SigSecp256k1)
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
		return ErrNotFound
	}

	os := new(types.OrderSeq)
	err = cbor.Unmarshal(resp.GetData().GetMsgInfo(), os)
	if err != nil {
		return err
	}

	psig := new(types.Signature)
	err = psig.Deserialize(resp.GetData().GetSign())
	if err != nil {
		return err
	}

	pmsg := blake3.Sum256(resp.GetData().GetMsgInfo())
	ok := m.RoleVerify(proID, pmsg[:], *psig)
	if ok {
		osp := &orderSeqPro{
			proID: proID,
			os:    os,
		}
		m.seqFinishChan <- osp
	}

	return nil
}
