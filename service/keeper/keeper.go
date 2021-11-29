package keeper

import (
	"context"
	"sync"
	"time"

	"github.com/memoio/go-mefs-v2/api"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/submodule/node"
	"github.com/memoio/go-mefs-v2/submodule/txPool"
)

var logger = logging.Logger("keeper")

var _ api.FullNode = (*KeeperNode)(nil)

type KeeperNode struct {
	sync.RWMutex

	*node.BaseNode

	ctx context.Context

	inp *txPool.InPool
}

func New(ctx context.Context, opts ...node.BuilderOpt) (*KeeperNode, error) {
	bn, err := node.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	inp := txPool.NewInPool(ctx, bn.PPool.SyncPool)

	// register for can apply msg
	inp.RegisterValidateMsgFunc(bn.StateDB.ValidateMsg)
	inp.RegisterValidateBlockFunc(bn.StateDB.ValidateBlock)

	kn := &KeeperNode{
		BaseNode: bn,
		ctx:      ctx,
		inp:      inp,
	}

	return kn, nil
}

// start service related
func (k *KeeperNode) Start() error {
	go k.OpenTest()

	// register net msg handle
	k.GenericService.Register(pb.NetMessage_SayHello, k.DefaultHandler)
	k.GenericService.Register(pb.NetMessage_Get, k.HandleGet)

	k.TxMsgHandle.Register(k.txMsgHandler)
	k.BlockHandle.Register(k.BaseNode.TxBlockHandler)

	k.RPCServer.Register("Memoriae", api.PermissionedFullAPI(k))

	// wait for sync

	for {
		if k.PPool.Ready() {
			break
		} else {
			logger.Debug("wait for sync")
			time.Sleep(5 * time.Second)
		}
	}

	logger.Info("start keeper for: ", k.RoleID())
	return nil
}

func (k *KeeperNode) RunDaemon(ready chan interface{}) error {
	return k.BaseNode.RunDaemon(ready)
}

func (k *KeeperNode) Close() {
	k.BaseNode.Close()
}
