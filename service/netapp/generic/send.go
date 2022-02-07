package generic

import (
	"context"

	"github.com/libp2p/go-libp2p-core/network"
	peer "github.com/libp2p/go-libp2p-core/peer"

	"github.com/memoio/go-mefs-v2/lib/pb"
)

// send
func (gs *GenericService) SendNetRequest(ctx context.Context, p peer.ID, id uint64, typ pb.NetMessage_MsgType, value, sig []byte) (*pb.NetMessage, error) {
	if gs.ns.Host.Network().Connectedness(p) != network.Connected {
		pai := peer.AddrInfo{
			ID: p,
		}

		err := gs.ns.NetConnect(ctx, pai)
		if err != nil {
			return nil, err
		}
	}

	nm := &pb.NetMessage{
		Header: &pb.NetMessage_MsgHeader{
			From: id,
			Type: typ,
		},
		Data: &pb.NetMessage_MsgData{
			MsgInfo: value,
			Sign:    sig,
		},
	}

	return gs.msgSender.SendRequest(ctx, p, nm)
}
