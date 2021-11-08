package provider

import (
	"context"
	"sync"

	"github.com/memoio/go-mefs-v2/api"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/submodule/node"
)

var logger = logging.Logger("provider")

var _ api.FullNode = (*ProviderNode)(nil)

type ProviderNode struct {
	sync.RWMutex

	*node.BaseNode

	segStore segment.SegmentStore

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

	kn := &ProviderNode{
		BaseNode: bn,
		ctx:      ctx,
		segStore: segStore,
	}

	return kn, nil
}

// start service related
func (p *ProviderNode) Start() error {
	// register net msg handle
	p.GenericService.Register(pb.NetMessage_Get, p.defaultHandler)

	p.TxMsgHandle.Register(tx.DataTxErr, p.defaultPubsubHandler)

	p.RPCServer.Register("Memoriae", api.PermissionedFullAPI(p))

	logger.Info("start keeper for: ", p.RoleID())
	return nil
}

func (p *ProviderNode) RunDaemon(ready chan interface{}) error {
	return p.BaseNode.RunDaemon(ready)
}

func (p *ProviderNode) Close() {
	p.BaseNode.Close()
}
