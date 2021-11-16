package provider

import (
	"context"
	"sync"

	"github.com/memoio/go-mefs-v2/api"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/service/data"
	porder "github.com/memoio/go-mefs-v2/service/provider/order"
	"github.com/memoio/go-mefs-v2/submodule/node"
)

var logger = logging.Logger("provider")

var _ api.FullNode = (*ProviderNode)(nil)

type ProviderNode struct {
	sync.RWMutex

	api.IDataService

	*node.BaseNode

	*porder.OrderMgr

	ctx context.Context
}

func New(ctx context.Context, opts ...node.BuilderOpt) (*ProviderNode, error) {
	bn, err := node.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	segStore, err := segment.NewSegStore(bn.Repo.FileStore())
	if err != nil {
		return nil, err
	}

	ids := data.New(bn.MetaStore(), segStore, bn.NetServiceImpl)

	por := porder.NewOrderMgr(ctx, bn.RoleID(), bn.MetaStore(), bn.RoleMgr, bn.NetServiceImpl, ids)

	kn := &ProviderNode{
		BaseNode:     bn,
		IDataService: ids,
		ctx:          ctx,
		OrderMgr:     por,
	}

	return kn, nil
}

// start service related
func (p *ProviderNode) Start() error {
	go p.OpenTest()

	// register net msg handle
	p.GenericService.Register(pb.NetMessage_SayHello, p.DefaultHandler)

	p.GenericService.Register(pb.NetMessage_Get, p.HandleGet)

	p.GenericService.Register(pb.NetMessage_AskPrice, p.handleQuotation)
	p.GenericService.Register(pb.NetMessage_CreateOrder, p.handleCreateOrder)
	p.GenericService.Register(pb.NetMessage_CreateSeq, p.handleCreateSeq)
	p.GenericService.Register(pb.NetMessage_FinishSeq, p.handleFinishSeq)
	p.GenericService.Register(pb.NetMessage_OrderSegment, p.handleSegData)

	p.TxMsgHandle.Register(tx.DataTxErr, p.DefaultPubsubHandler)

	p.RPCServer.Register("Memoriae", api.PermissionedFullAPI(p))

	logger.Info("start provider for: ", p.RoleID())
	return nil
}

func (p *ProviderNode) RunDaemon(ready chan interface{}) error {
	return p.BaseNode.RunDaemon(ready)
}

func (p *ProviderNode) Close() {
	p.BaseNode.Close()
}
