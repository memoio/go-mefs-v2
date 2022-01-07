package user

import (
	"context"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
)

// todo: need?
func (u *UserNode) handleGetSeg(ctx context.Context, pid peer.ID, mes *pb.NetMessage) (*pb.NetMessage, error) {
	logger.Debug("handle get segment from:", mes.GetHeader().From)
	resp := &pb.NetMessage{
		Header: &pb.NetMessage_MsgHeader{
			Version: 1,
			Type:    mes.GetHeader().GetType(),
			From:    u.RoleID(),
		},
		Data: &pb.NetMessage_MsgData{},
	}

	segID, err := segment.FromBytes(mes.GetData().GetMsgInfo())
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		resp.Data.MsgInfo = []byte(err.Error())
		return resp, nil
	}

	seg, err := u.GetSegmentFromLocal(ctx, segID)
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		resp.Data.MsgInfo = []byte(err.Error())
		return resp, nil
	}

	data, err := seg.Serialize()
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		resp.Data.MsgInfo = []byte(err.Error())
		return resp, nil
	}

	resp.Data.MsgInfo = data

	return resp, nil
}
