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
	"github.com/memoio/go-mefs-v2/lib/utils"
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

	*porder.OrderMgr

	chalSeg *pchal.SegMgr

	rp *readpay.ReceivePay

	ctx context.Context

	orderService bool // false when no available space or stop in config
	ready        bool
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

	sm := pchal.NewSegMgr(ctx, bn.RoleID(), ds, ids, bn)

	oc := bn.Repo.Config().Order

	por := porder.NewOrderMgr(ctx, bn.RoleID(), oc.Price, ds, bn.RoleMgr, bn.NetServiceImpl, ids, bn, bn.ISettle)

	rp := readpay.NewReceivePay(localAddr, ds)

	pn := &ProviderNode{
		BaseNode:     bn,
		IDataService: ids,
		ctx:          ctx,
		OrderMgr:     por,
		chalSeg:      sm,
		rp:           rp,
	}

	return pn, nil
}

// start service related
func (p *ProviderNode) Start(perm bool) error {
	p.Perm = perm

	p.RoleType = "provider"
	err := p.BaseNode.StartLocal()
	if err != nil {
		return err
	}

	// register net msg handle
	p.GenericService.Register(pb.NetMessage_AskPrice, p.handleQuotation)
	p.GenericService.Register(pb.NetMessage_CreateOrder, p.handleCreateOrder)
	p.GenericService.Register(pb.NetMessage_CreateSeq, p.handleCreateSeq)
	p.GenericService.Register(pb.NetMessage_FinishSeq, p.handleFinishSeq)

	p.GenericService.Register(pb.NetMessage_PutSegment, p.handleSegData)
	p.GenericService.Register(pb.NetMessage_GetSegment, p.handleGetSeg)

	if p.Perm {
		p.RPCServer.Register("Memoriae", api.PermissionedProviderAPI(metrics.MetricedProviderAPI(p)))
	} else {
		p.RPCServer.Register("Memoriae", metrics.MetricedProviderAPI(p))
	}

	go func() {
		p.BaseNode.WaitForSync()

		// wait for register
		err := p.Register()
		if err != nil {
			panic(err)
		}

		err = p.UpdateNetAddr()
		if err != nil {
			panic(err)
		}

		// start order manager
		p.OrderMgr.Start()

		// start challenge manager
		p.chalSeg.Start()

		p.ready = true
	}()

	go p.check()

	logger.Info("start provider: ", p.RoleID())
	return nil
}

func (p *ProviderNode) Ready(ctx context.Context) bool {
	return p.orderService && p.ready
}

func (p *ProviderNode) check() {
	if p.Repo.Config().Order.Stop {
		p.orderService = false
		return
	}

	ds := p.Repo.FileStore().Size()
	if ds.Free > utils.GiB {
		p.orderService = true
	}

	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			ds := p.Repo.FileStore().Size()
			if ds.Free < utils.GiB {
				logger.Debug("order stop due to low space")
				p.orderService = false
			}

			if ds.Free > 2*utils.GiB {
				logger.Debug("order start due to avail space")
				p.orderService = true
			}
		}
	}
}
