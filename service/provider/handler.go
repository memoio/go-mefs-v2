package provider

import (
	"context"
	"log"

	peer "github.com/libp2p/go-libp2p-core/peer"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/zeebo/blake3"
)

func (p *ProviderNode) defaultHandler(ctx context.Context, pid peer.ID, mes *pb.NetMessage) (*pb.NetMessage, error) {
	mes.Data.MsgInfo = []byte("hello")
	return mes, nil
}

func (p *ProviderNode) defaultPubsubHandler(ctx context.Context, mes *tx.SignedMessage) error {
	log.Println("keeper received pub msg:", mes.Method, mes.From)
	return nil
}

func (p *ProviderNode) handleGet(ctx context.Context, pid peer.ID, mes *pb.NetMessage) (*pb.NetMessage, error) {
	resp := &pb.NetMessage{
		Header: &pb.NetMessage_MsgHeader{
			Version: 1,
			From:    p.RoleID(),
		},
		Data: &pb.NetMessage_MsgData{},
	}

	key := mes.Data.MsgInfo
	val, err := p.MetaStore().Get(key)
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		return resp, nil
	}
	resp.Data.MsgInfo = val
	return resp, nil
}

func (p *ProviderNode) handleQuotation(ctx context.Context, pid peer.ID, mes *pb.NetMessage) (*pb.NetMessage, error) {
	resp := &pb.NetMessage{
		Header: &pb.NetMessage_MsgHeader{
			Version: 1,
			From:    p.RoleID(),
		},
		Data: &pb.NetMessage_MsgData{},
	}

	// verify sig
	res, err := p.HandleQuotation(mes.Header.From)
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		return resp, nil
	}

	resp.Data.MsgInfo = res

	msg := blake3.Sum256(res)

	sigTo, err := p.RoleMgr.RoleSign(msg[:], types.SigSecp256k1)
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		return resp, nil
	}

	sigByte, err := sigTo.Serialize()
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		return resp, nil
	}

	resp.Data.Sign = sigByte

	return resp, nil
}

func (p *ProviderNode) handleSegData(ctx context.Context, pid peer.ID, mes *pb.NetMessage) (*pb.NetMessage, error) {
	resp := &pb.NetMessage{
		Header: &pb.NetMessage_MsgHeader{
			Version: 1,
			From:    p.RoleID(),
		},
		Data: &pb.NetMessage_MsgData{},
	}

	// verify sig
	sigFrom := new(types.Signature)
	err := sigFrom.Deserialize(mes.GetData().GetSign())
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		return resp, nil
	}

	dataFrom := mes.GetData().GetMsgInfo()
	msgFrom := blake3.Sum256(dataFrom)

	ok := p.RoleMgr.RoleVerify(mes.Header.From, msgFrom[:], *sigFrom)
	if !ok {
		resp.Header.Type = pb.NetMessage_Err
		return resp, nil
	}

	seg := new(segment.BaseSegment)
	err = seg.Deserialize(dataFrom)
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		return resp, nil
	}

	err = p.PutSegmentToLocal(ctx, seg)
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		return resp, nil
	}

	err = p.HandleData(mes.Header.From, seg)
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		return resp, nil
	}

	return resp, nil
}

func (p *ProviderNode) handleCreateOrder(ctx context.Context, pid peer.ID, mes *pb.NetMessage) (*pb.NetMessage, error) {
	resp := &pb.NetMessage{
		Header: &pb.NetMessage_MsgHeader{
			Version: 1,
			From:    p.RoleID(),
		},
		Data: &pb.NetMessage_MsgData{},
	}

	// verify sig
	sigFrom := new(types.Signature)
	err := sigFrom.Deserialize(mes.GetData().GetSign())
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		return resp, nil
	}

	dataFrom := mes.GetData().GetMsgInfo()
	msgFrom := blake3.Sum256(dataFrom)

	ok := p.RoleMgr.RoleVerify(mes.Header.From, msgFrom[:], *sigFrom)
	if !ok {
		resp.Header.Type = pb.NetMessage_Err
		return resp, nil
	}

	res, err := p.HandleCreateOrder(dataFrom)
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		return resp, nil
	}

	resp.Data.MsgInfo = res

	msg := blake3.Sum256(res)

	sigTo, err := p.RoleMgr.RoleSign(msg[:], types.SigSecp256k1)
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		return resp, nil
	}

	sigByte, err := sigTo.Serialize()
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		return resp, nil
	}

	resp.Data.Sign = sigByte

	return resp, nil
}

func (p *ProviderNode) handleCreateSeq(ctx context.Context, pid peer.ID, mes *pb.NetMessage) (*pb.NetMessage, error) {
	resp := &pb.NetMessage{
		Header: &pb.NetMessage_MsgHeader{
			Version: 1,
			From:    p.RoleID(),
		},
		Data: &pb.NetMessage_MsgData{},
	}

	// verify sig
	sigFrom := new(types.Signature)
	err := sigFrom.Deserialize(mes.GetData().GetSign())
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		return resp, nil
	}

	dataFrom := mes.GetData().GetMsgInfo()
	msgFrom := blake3.Sum256(dataFrom)

	ok := p.RoleMgr.RoleVerify(mes.Header.From, msgFrom[:], *sigFrom)
	if !ok {
		resp.Header.Type = pb.NetMessage_Err
		return resp, nil
	}

	res, err := p.HandleCreateSeq(mes.Header.From, dataFrom)
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		return resp, nil
	}

	resp.Data.MsgInfo = res

	msg := blake3.Sum256(res)

	sigTo, err := p.RoleMgr.RoleSign(msg[:], types.SigSecp256k1)
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		return resp, nil
	}

	sigByte, err := sigTo.Serialize()
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		return resp, nil
	}

	resp.Data.Sign = sigByte

	return resp, nil
}

func (p *ProviderNode) handleFinishSeq(ctx context.Context, pid peer.ID, mes *pb.NetMessage) (*pb.NetMessage, error) {
	resp := &pb.NetMessage{
		Header: &pb.NetMessage_MsgHeader{
			Version: 1,
			From:    p.RoleID(),
		},
		Data: &pb.NetMessage_MsgData{},
	}

	// verify sig
	sigFrom := new(types.Signature)
	err := sigFrom.Deserialize(mes.GetData().GetSign())
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		return resp, nil
	}

	dataFrom := mes.GetData().GetMsgInfo()
	msgFrom := blake3.Sum256(dataFrom)

	ok := p.RoleMgr.RoleVerify(mes.Header.From, msgFrom[:], *sigFrom)
	if !ok {
		resp.Header.Type = pb.NetMessage_Err
		return resp, nil
	}

	res, err := p.HandleFinishSeq(mes.Header.From, dataFrom)
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		return resp, nil
	}

	resp.Data.MsgInfo = res

	msg := blake3.Sum256(res)

	sigTo, err := p.RoleMgr.RoleSign(msg[:], types.SigSecp256k1)
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		return resp, nil
	}

	sigByte, err := sigTo.Serialize()
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		return resp, nil
	}

	resp.Data.Sign = sigByte

	return resp, nil
}
