package provider

import (
	"context"
	"math/big"

	peer "github.com/libp2p/go-libp2p-core/peer"
	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/submodule/connect/readpay"
	"github.com/zeebo/blake3"
)

func (p *ProviderNode) handleGetSeg(ctx context.Context, pid peer.ID, mes *pb.NetMessage) (*pb.NetMessage, error) {
	logger.Debug("handle get segment from:", mes.GetHeader().From)
	resp := &pb.NetMessage{
		Header: &pb.NetMessage_MsgHeader{
			Version: 1,
			Type:    mes.GetHeader().GetType(),
			From:    p.RoleID(),
		},
		Data: &pb.NetMessage_MsgData{},
	}

	pc := new(readpay.Paycheck)
	err := pc.Deserialize(mes.GetData().Sign)
	if err != nil {
		logger.Debug("readpay fail:", err)
		resp.Header.Type = pb.NetMessage_Err
		resp.Data.MsgInfo = []byte(err.Error())
		return resp, nil
	}

	readPrice := big.NewInt(types.DefaultReadPrice)
	readPrice.Mul(readPrice, big.NewInt(build.DefaultSegSize))
	err = p.rp.Verify(pc, readPrice)
	if err != nil {
		logger.Debug("readpay fail:", err)
		resp.Header.Type = pb.NetMessage_Err
		resp.Data.MsgInfo = []byte(err.Error())
		return resp, nil
	}

	segID, err := segment.FromBytes(mes.GetData().GetMsgInfo())
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		resp.Data.MsgInfo = []byte(err.Error())
		return resp, nil
	}

	seg, err := p.GetSegmentFromLocal(ctx, segID)
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

func (p *ProviderNode) handleQuotation(ctx context.Context, pid peer.ID, mes *pb.NetMessage) (*pb.NetMessage, error) {
	logger.Debug("handle quotation from:", mes.GetHeader().From)
	resp := &pb.NetMessage{
		Header: &pb.NetMessage_MsgHeader{
			Version: 1,
			Type:    mes.GetHeader().GetType(),
			From:    p.RoleID(),
		},
		Data: &pb.NetMessage_MsgData{},
	}

	if !p.ready {
		logger.Debug("pro service not ready, fail handle quotation from:", mes.GetHeader().From)
		resp.Header.Type = pb.NetMessage_Err
		resp.Data.MsgInfo = []byte("service not ready")
		return resp, nil
	}

	res, err := p.OrderMgr.HandleQuotation(mes.Header.From)
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		resp.Data.MsgInfo = []byte(err.Error())
		return resp, nil
	}

	resp.Data.MsgInfo = res

	msg := blake3.Sum256(res)

	sigTo, err := p.RoleMgr.RoleSign(p.ctx, p.RoleID(), msg[:], types.SigSecp256k1)
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		resp.Data.MsgInfo = []byte(err.Error())
		return resp, nil
	}

	sigByte, err := sigTo.Serialize()
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		resp.Data.MsgInfo = []byte(err.Error())
		return resp, nil
	}

	resp.Data.Sign = sigByte

	return resp, nil
}

func (p *ProviderNode) handleSegData(ctx context.Context, pid peer.ID, mes *pb.NetMessage) (*pb.NetMessage, error) {
	logger.Debug("handle segdata from:", mes.GetHeader().From)
	resp := &pb.NetMessage{
		Header: &pb.NetMessage_MsgHeader{
			Version: 1,
			Type:    mes.GetHeader().GetType(),
			From:    p.RoleID(),
		},
		Data: &pb.NetMessage_MsgData{},
	}

	// verify sig; need it for security
	/*
		sigFrom := new(types.Signature)
		err := sigFrom.Deserialize(mes.GetData().GetSign())
		if err != nil {
			resp.Header.Type = pb.NetMessage_Err
			return resp, nil
		}

		dataFrom := mes.GetData().GetMsgInfo()
		msgFrom := blake3.Sum256(dataFrom)

		ok, _ := p.RoleMgr.RoleVerify(p.ctx, mes.Header.From, msgFrom[:], *sigFrom)
		if !ok {
			resp.Header.Type = pb.NetMessage_Err
			return resp, nil
		}
	*/

	seg := new(segment.BaseSegment)
	err := seg.Deserialize(mes.GetData().GetMsgInfo())
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		resp.Data.MsgInfo = []byte(err.Error())
		return resp, nil
	}

	err = seg.IsValid(build.DefaultSegSize)
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		resp.Data.MsgInfo = []byte(err.Error())
		return resp, nil
	}

	err = p.OrderMgr.HandleData(mes.Header.From, seg)
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		resp.Data.MsgInfo = []byte(err.Error())
		return resp, nil
	}

	return resp, nil
}

func (p *ProviderNode) handleCreateOrder(ctx context.Context, pid peer.ID, mes *pb.NetMessage) (*pb.NetMessage, error) {
	logger.Debug("handle create order from:", mes.GetHeader().From)
	resp := &pb.NetMessage{
		Header: &pb.NetMessage_MsgHeader{
			Version: 1,
			Type:    mes.GetHeader().GetType(),
			From:    p.RoleID(),
		},
		Data: &pb.NetMessage_MsgData{},
	}

	if !p.ready {
		logger.Debug("pro service not ready, fail handle create order from:", mes.GetHeader().From)
		resp.Header.Type = pb.NetMessage_Err
		resp.Data.MsgInfo = []byte("pro service not ready")
		return resp, nil
	}

	// verify sig
	sigFrom := new(types.Signature)
	err := sigFrom.Deserialize(mes.GetData().GetSign())
	if err != nil {
		logger.Debug("fail handle create order from:", mes.GetHeader().From, err)
		resp.Header.Type = pb.NetMessage_Err
		resp.Data.MsgInfo = []byte(err.Error())
		return resp, nil
	}

	dataFrom := mes.GetData().GetMsgInfo()
	msgFrom := blake3.Sum256(dataFrom)

	ok, err := p.RoleMgr.RoleVerify(p.ctx, mes.Header.From, msgFrom[:], *sigFrom)
	if !ok {
		logger.Debug("fail handle create order duo to verify from:", mes.GetHeader().From)
		resp.Header.Type = pb.NetMessage_Err
		if err != nil {
			resp.Data.MsgInfo = []byte(err.Error())
		}
		return resp, nil
	}

	res, err := p.OrderMgr.HandleCreateOrder(dataFrom)
	if err != nil {
		logger.Debug("fail handle create order from:", mes.GetHeader().From, err)
		resp.Header.Type = pb.NetMessage_Err
		resp.Data.MsgInfo = []byte(err.Error())
		return resp, nil
	}

	resp.Data.MsgInfo = res

	msg := blake3.Sum256(res)

	sigTo, err := p.RoleMgr.RoleSign(p.ctx, p.RoleID(), msg[:], types.SigSecp256k1)
	if err != nil {
		logger.Debug("fail handle create order from:", mes.GetHeader().From, err)
		resp.Header.Type = pb.NetMessage_Err
		resp.Data.MsgInfo = []byte(err.Error())
		return resp, nil
	}

	sigByte, err := sigTo.Serialize()
	if err != nil {
		logger.Debug("fail handle create order from:", mes.GetHeader().From, err)
		resp.Header.Type = pb.NetMessage_Err
		resp.Data.MsgInfo = []byte(err.Error())
		return resp, nil
	}

	resp.Data.Sign = sigByte

	return resp, nil
}

func (p *ProviderNode) handleCreateSeq(ctx context.Context, pid peer.ID, mes *pb.NetMessage) (*pb.NetMessage, error) {
	logger.Debug("handle create seq from:", mes.GetHeader().From)
	resp := &pb.NetMessage{
		Header: &pb.NetMessage_MsgHeader{
			Version: 1,
			Type:    mes.GetHeader().GetType(),
			From:    p.RoleID(),
		},
		Data: &pb.NetMessage_MsgData{},
	}

	// verify sig
	sigFrom := new(types.Signature)
	err := sigFrom.Deserialize(mes.GetData().GetSign())
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		resp.Data.MsgInfo = []byte(err.Error())
		return resp, nil
	}

	dataFrom := mes.GetData().GetMsgInfo()
	msgFrom := blake3.Sum256(dataFrom)

	ok, err := p.RoleMgr.RoleVerify(p.ctx, mes.Header.From, msgFrom[:], *sigFrom)
	if !ok {
		resp.Header.Type = pb.NetMessage_Err
		if err != nil {
			resp.Data.MsgInfo = []byte(err.Error())
		}
		return resp, nil
	}

	res, err := p.OrderMgr.HandleCreateSeq(mes.Header.From, dataFrom)
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		resp.Data.MsgInfo = []byte(err.Error())
		return resp, nil
	}

	resp.Data.MsgInfo = res

	msg := blake3.Sum256(res)

	sigTo, err := p.RoleMgr.RoleSign(p.ctx, p.RoleID(), msg[:], types.SigSecp256k1)
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		resp.Data.MsgInfo = []byte(err.Error())
		return resp, nil
	}

	sigByte, err := sigTo.Serialize()
	if err != nil {
		resp.Header.Type = pb.NetMessage_Err
		resp.Data.MsgInfo = []byte(err.Error())
		return resp, nil
	}

	resp.Data.Sign = sigByte

	return resp, nil
}

func (p *ProviderNode) handleFinishSeq(ctx context.Context, pid peer.ID, mes *pb.NetMessage) (*pb.NetMessage, error) {
	logger.Debug("handle finish seq from:", mes.GetHeader().From)
	resp := &pb.NetMessage{
		Header: &pb.NetMessage_MsgHeader{
			Version: 1,
			Type:    mes.GetHeader().GetType(),
			From:    p.RoleID(),
		},
		Data: &pb.NetMessage_MsgData{},
	}

	// verify sig
	sigFrom := new(types.Signature)
	err := sigFrom.Deserialize(mes.GetData().GetSign())
	if err != nil {
		logger.Debug("fail handle finish seq from:", mes.GetHeader().From, err)
		resp.Header.Type = pb.NetMessage_Err
		resp.Data.MsgInfo = []byte(err.Error())
		return resp, nil
	}

	dataFrom := mes.GetData().GetMsgInfo()
	msgFrom := blake3.Sum256(dataFrom)

	ok, err := p.RoleMgr.RoleVerify(p.ctx, mes.Header.From, msgFrom[:], *sigFrom)
	if !ok {
		logger.Debug("fail handle finish seq from:", mes.GetHeader().From)
		resp.Header.Type = pb.NetMessage_Err
		if err != nil {
			resp.Data.MsgInfo = []byte(err.Error())
		}
		return resp, nil
	}

	res, err := p.OrderMgr.HandleFinishSeq(mes.Header.From, dataFrom)
	if err != nil {
		logger.Debug("fail handle finish seq from:", mes.GetHeader().From, err)
		resp.Header.Type = pb.NetMessage_Err
		resp.Data.MsgInfo = []byte(err.Error())
		return resp, nil
	}

	resp.Data.MsgInfo = res

	msg := blake3.Sum256(res)

	sigTo, err := p.RoleMgr.RoleSign(p.ctx, p.RoleID(), msg[:], types.SigSecp256k1)
	if err != nil {
		logger.Debug("fail handle finish seq from:", mes.GetHeader().From, err)
		resp.Header.Type = pb.NetMessage_Err
		resp.Data.MsgInfo = []byte(err.Error())
		return resp, nil
	}

	sigByte, err := sigTo.Serialize()
	if err != nil {
		logger.Debug("fail handle finish seq from:", mes.GetHeader().From, err)
		resp.Header.Type = pb.NetMessage_Err
		resp.Data.MsgInfo = []byte(err.Error())
		return resp, nil
	}

	resp.Data.Sign = sigByte

	return resp, nil
}
