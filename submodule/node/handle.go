package node

import (
	"context"
	"time"

	"github.com/gogo/protobuf/proto"

	peer "github.com/libp2p/go-libp2p-core/peer"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
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
	return mes, nil
}

func (n *BaseNode) HandleGet(ctx context.Context, pid peer.ID, mes *pb.NetMessage) (*pb.NetMessage, error) {
	logger.Debug("handle get net message from: ", pid.Pretty())
	resp := &pb.NetMessage{
		Header: &pb.NetMessage_MsgHeader{
			Version: 1,
			Type:    mes.GetHeader().GetType(),
			From:    n.RoleID(),
		},
		Data: &pb.NetMessage_MsgData{},
	}

	key := mes.Data.MsgInfo
	val, err := n.MetaStore().Get(key)
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		return resp, nil
	}

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

	resp.Data.MsgInfo = val

	return resp, nil
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
				time.Sleep(5 * time.Second)
				continue
			}

			ctx, cancle := context.WithTimeout(n.ctx, 10*time.Minute)
			defer cancle()
			for {
				st, err := n.PushPool.GetTxMsgStatus(ctx, mid)
				if err != nil {
					time.Sleep(10 * time.Second)
					continue
				}

				logger.Debug("tx message done: ", mid, st.BlockID, st.Height, st.Status.Err, string(st.Status.Extra))
				break
			}
			break
		}
	}

	logger.Debug("role is registered")

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
	go n.NetServiceImpl.AddNode(ri.ID, p)

	resp := new(pb.NetMessage)

	return resp, nil
}
