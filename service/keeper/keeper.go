package keeper

import (
	"context"
	"sync"

	"github.com/memoio/go-mefs-v2/api"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/pb"
	bcommon "github.com/memoio/go-mefs-v2/submodule/consensus/common"
	"github.com/memoio/go-mefs-v2/submodule/consensus/hotstuff"
	"github.com/memoio/go-mefs-v2/submodule/consensus/poa"
	"github.com/memoio/go-mefs-v2/submodule/metrics"
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

	bc bcommon.ConsensusMgr

	inProcess bool
	ready     bool
}

func New(ctx context.Context, opts ...node.BuilderOpt) (*KeeperNode, error) {
	bn, err := node.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	inp := txPool.NewInPool(ctx, bn.RoleID(), bn.LPP.SyncPool)

	kn := &KeeperNode{
		BaseNode: bn,
		ctx:      ctx,
		inp:      inp,
	}

	thr, err := bn.StateGetThreshold(ctx)
	if err != nil {
		return nil, err
	}
	if thr == 1 {
		kn.bc = poa.NewPoAManager(ctx, bn.RoleID(), bn.RoleMgr, bn.NetServiceImpl, inp)
	} else {
		hm := hotstuff.NewHotstuffManager(ctx, bn.RoleID(), bn.RoleMgr, bn.NetServiceImpl, inp)

		kn.bc = hm
		kn.HsMsgHandle.Register(hm.HandleMessage)
	}

	return kn, nil
}

// start service related
func (k *KeeperNode) Start(perm bool) error {
	k.Perm = perm
	k.RoleType = "keeper"

	err := k.BaseNode.StartLocal()
	if err != nil {
		return err
	}

	// register net msg handle

	// handle received tx message
	k.TxMsgHandle.Register(k.txMsgHandler)

	// handle event message; later
	k.EventHandle.Register(pb.EventMessage_LfsMeta, k.putLfsMetaHandler)

	if k.Perm {
		k.RPCServer.Register("Memoriae", api.PermissionedFullAPI(metrics.MetricedKeeperAPI(k)))
	} else {
		k.RPCServer.Register("Memoriae", metrics.MetricedKeeperAPI(k))
	}

	go func() {
		k.inp.Start()

		go k.bc.MineBlock()

		err := k.Register()
		if err != nil {
			panic(err)
		}

		go k.updateChalEpoch()
		go k.updatePay()
		go k.updateOrder()
		k.ready = true
	}()

	logger.Info("start keeper: ", k.RoleID())
	return nil
}

func (k *KeeperNode) Ready(ctx context.Context) bool {
	return k.ready
}
