package keeper

import (
	"context"
	"sync"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/memoio/go-mefs-v2/api"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/submodule/node"
)

var logger = logging.Logger("basenode")

var _ api.FullNode = (*KeeperNode)(nil)

type KeeperNode struct {
	sync.RWMutex

	*node.BaseNode

	ctx context.Context
}

func New(ctx context.Context, opts ...node.BuilderOpt) (*KeeperNode, error) {
	bn, err := node.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	kn := &KeeperNode{
		BaseNode: bn,
		ctx:      ctx,
	}

	return kn, nil
}

// start service related
func (k *KeeperNode) Start() error {
	// register net msg handle
	k.GenericService.Register(pb.NetMessage_Get, k.defaultHandler)

	k.Handle.Register(tx.DataTxErr, k.defaultPubsubHandler)

	k.RPCServer = jsonrpc.NewServer()
	k.RPCServer.Register("Memoriae", api.PermissionedFullAPI(k))

	logger.Info("start keeper for: ", k.RoleID())
	return nil
}

func (k *KeeperNode) RunDaemon(ready chan interface{}) error {
	return k.BaseNode.RunDaemon(ready)
}

func (k *KeeperNode) Close() {
	k.BaseNode.Close()
}
