package order

import (
	"strings"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/zeebo/blake3"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
)

// handle msg over net, then dispatch to provider order
func (m *OrderMgr) runNetSched() {
	for {
		select {
		case pid := <-m.updateChan:
			logger.Debug("update pro time: ", pid)
			m.lk.RLock()
			of, ok := m.pInstMap[pid]
			m.lk.RUnlock()
			if ok {
				of.availTime = time.Now().Unix()
			} else {
				go m.createProOrder(pid)
			}
		case pid := <-m.failChan:
			logger.Debug("pro request fail: ", pid)
			m.lk.RLock()
			of, ok := m.pInstMap[pid]
			m.lk.RUnlock()
			if ok {
				of.failCnt += 10
			} else {
				go m.createProOrder(pid)
			}
		// handle order state
		case quo := <-m.quoChan:
			m.lk.RLock()
			of, ok := m.pInstMap[quo.ProID]
			m.lk.RUnlock()
			if ok {
				of.quoChan <- quo
			}
		case ob := <-m.orderChan:
			m.lk.RLock()
			of, ok := m.pInstMap[ob.ProID]
			m.lk.RUnlock()
			if ok {
				of.orderChan <- ob
			}
		case s := <-m.seqNewChan:
			m.lk.RLock()
			of, ok := m.pInstMap[s.proID]
			m.lk.RUnlock()
			if ok {
				of.seqNewChan <- s
			}
		case s := <-m.seqFinishChan:
			m.lk.RLock()
			of, ok := m.pInstMap[s.proID]
			m.lk.RUnlock()
			if ok {
				of.seqFinishChan <- s
			}
		case <-m.ctx.Done():
			return
		}
	}
}

func (m *OrderMgr) connect(proID uint64) error {
	logger.Debug("try connect pro: ", proID)
	// test remote service is ready or not
	resp, err := m.ns.SendMetaRequest(m.ctx, proID, pb.NetMessage_AskPrice, nil, nil)
	if err == nil {
		if resp.GetHeader().GetFrom() == proID && resp.GetHeader().GetType() != pb.NetMessage_Err {
			return nil
		}
	}

	logger.Debugf("try connect pro %d fail %s", proID, err)

	// otherwise get addr from declared address
	pi, err := m.StateGetNetInfo(m.ctx, proID)
	if err == nil {
		// todo: fix this
		err := m.ns.Host().Connect(m.ctx, pi)
		if err == nil {
			m.ns.Host().Peerstore().SetAddrs(pi.ID, pi.Addrs, time.Duration(24*time.Hour))
			m.ns.AddNode(proID, pi)
		} else {
			relay := false
			for _, maddr := range pi.Addrs {
				saddr := maddr.String()
				if strings.HasSuffix(saddr, "p2p-circuit") {
					relay = true
					saddr = strings.TrimSuffix(saddr, "p2p-circuit")
					rpai, err := peer.AddrInfoFromString(saddr)
					if err != nil {
						return err
					}

					err = m.ns.Host().Connect(m.ctx, *rpai)
					if err != nil {
						return err
					}

					rmaddr, err := ma.NewMultiaddr("/p2p/" + rpai.ID.Pretty() + "/p2p-circuit" + "/p2p/" + pi.ID.Pretty())
					if err != nil {
						return err
					}

					relayaddr := peer.AddrInfo{
						ID:    pi.ID,
						Addrs: []ma.Multiaddr{rmaddr},
					}

					err = m.ns.Host().Connect(m.ctx, relayaddr)
					if err != nil {
						return err
					}
				}
			}

			if !relay {
				logger.Debugf("try connect pro declared: %s %s", pi, err)
				return err
			}
		}

		logger.Debugf("try connect pro: %s %s", pi, err)
	}

	// test remote service is ready or not
	resp, err = m.ns.SendMetaRequest(m.ctx, proID, pb.NetMessage_AskPrice, nil, nil)
	if err != nil {
		logger.Warnf("send to pro %d %s", proID, err)
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
	if !m.RestrictHas(m.ctx, proID) {
		return
	}

	err := m.connect(proID)
	if err != nil {
		logger.Debug("fail connect: ", proID, err)
		return
	}

	m.updateChan <- proID
}

func (m *OrderMgr) getQuotation(proID uint64) error {
	logger.Debug("new quotation getr: ", proID)

	if !m.RestrictHas(m.ctx, proID) {
		return xerrors.Errorf("provider %d not in restrict list", proID)
	}

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
	ok, err := m.RoleVerify(m.ctx, proID, msg[:], *sig)
	if err != nil {
		logger.Debug("fail get new quotation from: ", proID, err)
		return err
	}

	if ok {
		logger.Debug("new quotation end getr: ", proID)
		m.quoChan <- quo
		logger.Debug("new quotation end1 getr: ", proID)
	}

	return nil
}

func (m *OrderMgr) getNewOrderAck(proID uint64, data []byte) error {
	logger.Debug("new order ack getr: ", proID)
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
		logger.Debug("fail get new order ack from: ", proID, err)
		return err
	}

	if resp.GetHeader().GetType() == pb.NetMessage_Err {
		m.failChan <- proID
		logger.Debug("fail get new order ack from: ", proID, string(resp.GetData().MsgInfo))
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
	ok, err := m.RoleVerify(m.ctx, proID, pmsg[:], *psig)
	if err != nil {
		logger.Debug("fail get new order ack from: ", proID, err)
		return err
	}
	if ok {
		logger.Debug("new order ack end getr: ", proID)
		m.orderChan <- ob
		logger.Debug("new order ack end1 getr: ", proID)
	}

	return nil
}

func (m *OrderMgr) getNewSeqAck(proID uint64, data []byte) error {
	logger.Debug("new seq ack getr: ", proID)
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
		logger.Debug("fail get new seq ack from: ", proID, err)
		return err
	}

	if resp.GetHeader().GetType() == pb.NetMessage_Err {
		m.failChan <- proID
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
	ok, err := m.RoleVerify(m.ctx, proID, pmsg[:], *psig)
	if err != nil {
		logger.Debug("fail get new seq ack from: ", proID, err)
		return err
	}

	if ok {
		logger.Debug("new seq ack end getr: ", proID)
		osp := &orderSeqPro{
			proID: proID,
			os:    os,
		}
		m.seqNewChan <- osp
		logger.Debug("new seq ack end1 getr: ", proID)
	}

	return nil
}

func (m *OrderMgr) getSeqFinishAck(proID uint64, data []byte) error {
	logger.Debug("finish seq ack getr: ", proID)
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
		logger.Debug("fail get finish seq ack from: ", proID, err)
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
	ok, err := m.RoleVerify(m.ctx, proID, pmsg[:], *psig)
	if err != nil {
		logger.Debug("fail get finish seq ack from: ", proID, err)
		return err
	}

	if ok {
		logger.Debug("finish seq ack end getr: ", proID)
		osp := &orderSeqPro{
			proID: proID,
			os:    os,
		}
		m.seqFinishChan <- osp
		logger.Debug("finish seq ack end1 getr: ", proID)
	}

	return nil
}

func (m *OrderMgr) getSeqFixAck(sos *types.SignedOrderSeq) (*types.SignedOrderSeq, error) {
	logger.Debug("fix seq ack getr: ", sos.ProID)
	data, err := sos.Serialize()
	if err != nil {
		return nil, err
	}

	msg := blake3.Sum256(data)
	sig, err := m.RoleSign(m.ctx, m.localID, msg[:], types.SigSecp256k1)
	if err != nil {
		return nil, err
	}

	sigByte, err := sig.Serialize()
	if err != nil {
		return nil, err
	}

	resp, err := m.ns.SendMetaRequest(m.ctx, sos.ProID, pb.NetMessage_FixSeq, data, sigByte)
	if err != nil {
		return nil, err
	}

	if resp.GetHeader().GetType() == pb.NetMessage_Err {
		return nil, xerrors.Errorf("get fix seq ack from %d fail %s", sos.ProID, resp.GetData().MsgInfo)
	}

	os := new(types.SignedOrderSeq)
	err = os.Deserialize(resp.GetData().GetMsgInfo())
	if err != nil {
		return nil, err
	}

	psig := new(types.Signature)
	err = psig.Deserialize(resp.GetData().GetSign())
	if err != nil {
		return nil, err
	}

	pmsg := blake3.Sum256(resp.GetData().GetMsgInfo())
	ok, err := m.RoleVerify(m.ctx, sos.ProID, pmsg[:], *psig)
	if err != nil {
		return nil, err
	}

	if !ok {
		return nil, xerrors.Errorf("get fix seq ack from %d wrong sign", sos.ProID)
	}

	return os, nil
}
