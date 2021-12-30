package provider

import (
	"context"
	"sync"
	"time"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/address"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/service/data"
	pchal "github.com/memoio/go-mefs-v2/service/provider/challenge"
	porder "github.com/memoio/go-mefs-v2/service/provider/order"
	"github.com/memoio/go-mefs-v2/submodule/connect/readpay"
	"github.com/memoio/go-mefs-v2/submodule/metrics"
	"github.com/memoio/go-mefs-v2/submodule/node"
)

var logger = logging.Logger("provider")

var _ api.ProviderNode = (*ProviderNode)(nil)

type ProviderNode struct {
	sync.RWMutex

	*node.BaseNode

	api.IDataService

	pom *porder.OrderMgr

	chalSeg *pchal.SegMgr

	rp *readpay.ReceivePay

	ctx context.Context

	ready bool
}

func New(ctx context.Context, opts ...node.BuilderOpt) (*ProviderNode, error) {
	bn, err := node.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	ds := bn.MetaStore()

	segStore, err := segment.NewSegStore(bn.Repo.FileStore())
	if err != nil {
		return nil, err
	}

	pri, err := bn.RoleMgr.RoleSelf(ctx)
	if err != nil {
		return nil, err
	}

	localAddr, err := address.NewAddress(pri.ChainVerifyKey)
	if err != nil {
		return nil, err
	}

	sp := readpay.NewSender(localAddr, bn.LocalWallet, ds)

	ids := data.New(ds, segStore, bn.NetServiceImpl, bn.RoleMgr, sp)

	sm := pchal.NewSegMgr(ctx, bn.RoleID(), ds, ids, bn.PushPool)

	por := porder.NewOrderMgr(ctx, bn.RoleID(), ds, bn.RoleMgr, bn.NetServiceImpl, ids, bn.PushPool)

	rp := readpay.NewReceivePay(localAddr, ds)

	pn := &ProviderNode{
		BaseNode:     bn,
		IDataService: ids,
		ctx:          ctx,
		pom:          por,
		chalSeg:      sm,
		rp:           rp,
	}

	return pn, nil
}

// start service related
func (p *ProviderNode) Start() error {
	if p.Repo.Config().Net.Name == "test" {
		go p.OpenTest()
	} else {
		p.RoleMgr.Start()
	}

	// register net msg handle
	p.GenericService.Register(pb.NetMessage_SayHello, p.DefaultHandler)
	p.GenericService.Register(pb.NetMessage_Get, p.HandleGet)

	p.GenericService.Register(pb.NetMessage_AskPrice, p.handleQuotation)
	p.GenericService.Register(pb.NetMessage_CreateOrder, p.handleCreateOrder)
	p.GenericService.Register(pb.NetMessage_CreateSeq, p.handleCreateSeq)
	p.GenericService.Register(pb.NetMessage_FinishSeq, p.handleFinishSeq)

	p.GenericService.Register(pb.NetMessage_PutSegment, p.handleSegData)
	p.GenericService.Register(pb.NetMessage_GetSegment, p.handleGetSeg)

	p.TxMsgHandle.Register(p.BaseNode.TxMsgHandler)
	p.BlockHandle.Register(p.BaseNode.TxBlockHandler)

	p.PushPool.RegisterAddUPFunc(p.chalSeg.AddUP)
	p.PushPool.RegisterDelSegFunc(p.chalSeg.RemoveSeg)

	p.RPCServer.Register("Memoriae", api.PermissionedProviderAPI(metrics.MetricedProviderAPI(p)))

	// wait for sync
	p.PushPool.Start()
	for {
		if p.PushPool.Ready() {
			break
		} else {
			logger.Debug("wait for sync")
			time.Sleep(5 * time.Second)
		}
	}

	// wait for register
	err := p.Register()
	if err != nil {
		return err
	}

	err = p.UpdateNetAddr()
	if err != nil {
		return err
	}

	// start challenge
	p.chalSeg.Start()

	p.ready = true

	logger.Info("start provider for: ", p.RoleID())
	return nil
}
