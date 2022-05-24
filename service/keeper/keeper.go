package keeper

import (
	"context"
	"net/http"
	"sync"
	"time"

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

	inp := txPool.NewInPool(ctx, bn.RoleID(), bn.PushPool.SyncPool)

	kn := &KeeperNode{
		BaseNode: bn,
		ctx:      ctx,
		inp:      inp,
	}

	if bn.GetThreshold(ctx) == 1 {
		kn.bc = poa.NewPoAManager(ctx, bn.RoleID(), bn.IRole, bn.INetService, inp)
	} else {
		hm := hotstuff.NewHotstuffManager(ctx, bn.RoleID(), bn.IRole, bn.INetService, inp)

		kn.bc = hm
		kn.HsMsgHandle.Register(hm.HandleMessage)
	}

	return kn, nil
}

// start service related
func (k *KeeperNode) Start(perm bool) error {
	k.Perm = perm

	if k.Repo.Config().Net.Name == "test" {
		go k.OpenTest()
	} else {
		k.RoleMgr.Start()
	}

	// register net msg handle
	k.GenericService.Register(pb.NetMessage_SayHello, k.DefaultHandler)
	k.GenericService.Register(pb.NetMessage_Get, k.HandleGet)

	k.TxMsgHandle.Register(k.txMsgHandler)
	k.BlockHandle.Register(k.BaseNode.TxBlockHandler)

	k.StateMgr.RegisterAddUserFunc(k.AddUsers)
	k.StateMgr.RegisterAddUPFunc(k.AddUP)

	k.HttpHandle.Handle("/debug/metrics", metrics.Exporter())
	k.HttpHandle.PathPrefix("/").Handler(http.DefaultServeMux)

	k.RPCServer.Register("Memoriae", api.PermissionedFullAPI(metrics.MetricedKeeperAPI(k)))

	go func() {
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

		go k.bc.MineBlock()

		err := k.Register()
		if err != nil {
			return
		}

		go k.updateChalEpoch()
		go k.updatePay()
		go k.updateOrder()
		k.ready = true
	}()

	logger.Info("Start keeper: ", k.RoleID())
	return nil
}

func (k *KeeperNode) Ready(ctx context.Context) bool {
	return k.ready
}
