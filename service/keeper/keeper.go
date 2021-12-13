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

	inp := txPool.NewInPool(ctx, bn.PushPool.SyncPool)

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

	k.StateMgr.RegisterAddUserFunc(k.AddUsers)
	k.StateMgr.RegisterAddUPFunc(k.AddUP)

	k.RPCServer.Register("Memoriae", api.PermissionedFullAPI(k))

	// wait for sync
	k.PushPool.Start()
	retry := 0
	for {
		if k.PushPool.Ready() {
			break
		} else {
			logger.Debug("wait for sync")
			retry++
			if retry > 12 {
				// no more new block, set to ready
				k.SyncPool.SetReady()
			}
			time.Sleep(5 * time.Second)
		}
	}

	k.inp.Start()

	err := k.Register()
	if err != nil {
		return err
	}

	go k.updateChalEpoch()
	go k.updatePay()

	logger.Info("start keeper for: ", k.RoleID())
	return nil
}

func (k *KeeperNode) Close() {
	k.BaseNode.Close()
}
