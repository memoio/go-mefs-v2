package node

import (
	"bytes"
	"context"
	"encoding/binary"
	"strings"
	"time"

	"github.com/gogo/protobuf/proto"
	peer "github.com/libp2p/go-libp2p-core/peer"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	ma "github.com/multiformats/go-multiaddr"
)

// todo: remove it
func (n *BaseNode) TxMsgHandler(ctx context.Context, mes *tx.SignedMessage) error {
	logger.Debug("received pub message:", mes.From, mes.Nonce, mes.Method)
	//return n.SyncPool.AddTxMsg(ctx, mes)
	return nil
}

func (n *BaseNode) TxBlockHandler(ctx context.Context, blk *tx.SignedBlock) error {
	logger.Debug("received pub block:", blk.MinerID, blk.Height)
	return n.SyncPool.AddTxBlock(blk)
}

func (n *BaseNode) DefaultHandler(ctx context.Context, pid peer.ID, mes *pb.NetMessage) (*pb.NetMessage, error) {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, n.RoleID())
	mes.Data.MsgInfo = buf

	return mes, nil
}

func (n *BaseNode) HandleGet(ctx context.Context, pid peer.ID, mes *pb.NetMessage) (*pb.NetMessage, error) {
	key := mes.Data.MsgInfo

	logger.Debug("handle get net message from: ", pid.Pretty(), ", key: ", string(key))
	resp := &pb.NetMessage{
		Header: &pb.NetMessage_MsgHeader{
			Version: 1,
			Type:    mes.GetHeader().GetType(),
			From:    n.RoleID(),
		},
		Data: &pb.NetMessage_MsgData{},
	}

	if bytes.HasPrefix(key, []byte("tx")) {
		val, err := n.StateStore().Get(key)
		if err != nil {
			resp.Header.Type = pb.NetMessage_Err
			resp.Data.MsgInfo = []byte(err.Error())
			return resp, nil
		}

		logger.Debug("handle get net message from ok: ", pid.Pretty(), ", key: ", string(key))

		resp.Data.MsgInfo = val
		return resp, nil
	}

	val, err := n.MetaStore().Get(key)
	if err == nil {
		logger.Debug("handle get net message from ok: ", pid.Pretty(), ", key: ", string(mes.Data.MsgInfo))
		resp.Data.MsgInfo = val
		return resp, nil
	}

	/*
		val, err = n.StateStore().Get(key)
		if err == nil {
			logger.Debug("handle get net message from ok: ", pid.Pretty(), ", key: ", string(mes.Data.MsgInfo))
			resp.Data.MsgInfo = val
			return resp, nil
		}

		ds := wrap.NewKVStore("tx", n.StateStore())
		val, err = ds.Get(key)
		if err == nil {
			logger.Debug("handle get net message from ok: ", pid.Pretty(), ", key: ", string(mes.Data.MsgInfo))
			resp.Data.MsgInfo = val
			return resp, nil
		}
	*/

	logger.Debug("handle get net message from no: ", pid.Pretty(), ", key: ", string(mes.Data.MsgInfo))
	resp.Header.Type = pb.NetMessage_Err
	resp.Data.MsgInfo = []byte(err.Error())
	return resp, nil

	/*
		msg := blake3.Sum256(val)
		sig, err := n.RoleSign(n.ctx, n.RoleID(), msg[:], types.SigSecp256k1)
		if err != nil {
			resp.Header.Type = pb.NetMessage_Err
			return resp, nil
		}

		sigByte, err := sig.Serialize()
		if err != nil {
			resp.Header.Type = pb.NetMessage_Err
			return resp, nil
		}
		resp.Data.Sign = sigByte
	*/

}

func (n *BaseNode) Register() error {
	_, err := n.PushPool.GetRoleBaseInfo(n.RoleID())
	if err != nil {
		ri, err := n.RoleSelf(n.ctx)
		if err != nil {
			return err
		}

		data, err := proto.Marshal(ri)
		if err != nil {
			return err
		}
		msg := &tx.Message{
			Version: 0,
			From:    n.RoleID(),
			To:      n.RoleID(),
			Method:  tx.AddRole,
			Params:  data,
		}

		for {
			mid, err := n.PushPool.PushMessage(n.ctx, msg)
			if err != nil {
				time.Sleep(30 * time.Second)
				continue
			}

			for {
				st, err := n.PushPool.SyncGetTxMsgStatus(n.ctx, mid)
				if err != nil {
					_, err := n.PushPool.GetRoleBaseInfo(n.RoleID())
					if err == nil {
						return nil
					}
					time.Sleep(10 * time.Second)
					continue
				}

				if st.Status.Err == 0 {
					logger.Debug("tx message done success: ", mid, msg.From, msg.To, msg.Method, st.BlockID, st.Height)
				} else {
					logger.Warn("tx message done fail: ", mid, msg.From, msg.To, msg.Method, st.BlockID, st.Height, st.Status)
				}
				break
			}
			break
		}
	}

	logger.Debug("role is registered")

	return nil
}

func (n *BaseNode) UpdateNetAddr() error {
	pi, err := n.NetAddrInfo(n.ctx)
	if err != nil {
		return err
	}

	opi, err := n.PushPool.StateGetNetInfo(n.ctx, n.RoleID())
	if err == nil {
		if pi.ID == opi.ID {
			logger.Debug("net addr has been on chain")
			return nil
		}
	}

	addrs := make([]ma.Multiaddr, 0, len(pi.Addrs))
	for _, maddr := range pi.Addrs {
		if strings.Contains(maddr.String(), "/127.0.0.1/") {
			continue
		}

		if strings.Contains(maddr.String(), "/::1/") {
			continue
		}

		addrs = append(addrs, maddr)
	}

	pi.Addrs = addrs
	data, err := pi.MarshalJSON()
	if err != nil {
		return err
	}
	msg := &tx.Message{
		Version: 0,
		From:    n.RoleID(),
		To:      n.RoleID(),
		Method:  tx.UpdateNet,
		Params:  data,
	}

	for {
		mid, err := n.PushPool.PushMessage(n.ctx, msg)
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}

		for {
			st, err := n.PushPool.SyncGetTxMsgStatus(n.ctx, mid)
			if err != nil {
				_, err := n.PushPool.StateGetNetInfo(n.ctx, n.RoleID())
				if err == nil {
					return nil
				}
				time.Sleep(10 * time.Second)
				continue
			}

			if st.Status.Err == 0 {
				logger.Debug("tx message done success: ", mid, msg.From, msg.To, msg.Method, st.BlockID, st.Height)
			} else {
				logger.Warn("tx message done fail: ", mid, msg.From, msg.To, msg.Method, st.BlockID, st.Height, st.Status)
			}
			break
		}
		break
	}

	logger.Debug("net addr is updated to: ", pi.String())

	return nil
}

func (n *BaseNode) OpenTest() error {
	ticker := time.NewTicker(11 * time.Second)
	defer ticker.Stop()
	pi, _ := n.RoleMgr.RoleSelf(n.ctx)
	data, _ := proto.Marshal(pi)
	n.MsgHandle.Register(pb.NetMessage_PutPeer, n.TestHanderPutPeer)

	for {
		select {
		case <-ticker.C:
			pinfos, err := n.NetworkSubmodule.NetPeers(n.ctx)
			if err == nil {
				for _, pi := range pinfos {
					n.GenericService.SendNetRequest(n.ctx, pi.ID, n.RoleID(), pb.NetMessage_PutPeer, data, nil)
				}
			}

		case <-n.ctx.Done():
			return nil
		}
	}
}

func (n *BaseNode) TestHanderPutPeer(ctx context.Context, p peer.ID, mes *pb.NetMessage) (*pb.NetMessage, error) {
	logger.Debugf("handle put peer msg from: %d, %s", mes.GetHeader().GetFrom(), p.Pretty())
	ri := new(pb.RoleInfo)
	err := proto.Unmarshal(mes.GetData().GetMsgInfo(), ri)
	if err != nil {
		return nil, err
	}

	go n.RoleMgr.AddRoleInfo(ri)
	go n.NetServiceImpl.AddNode(ri.RoleID, peer.AddrInfo{ID: p})

	resp := new(pb.NetMessage)

	return resp, nil
}
