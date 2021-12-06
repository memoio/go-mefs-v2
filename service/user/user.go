package user

import (
	"context"
	"sync"
	"time"

	"github.com/memoio/go-mefs-v2/api"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/service/data"
	"github.com/memoio/go-mefs-v2/service/user/lfs"
	uorder "github.com/memoio/go-mefs-v2/service/user/order"
	"github.com/memoio/go-mefs-v2/submodule/node"
)

var logger = logging.Logger("user")

var _ api.UserNode = (*UserNode)(nil)

type UserNode struct {
	sync.RWMutex

	*node.BaseNode

	*lfs.LfsService

	ctx context.Context
}

func New(ctx context.Context, opts ...node.BuilderOpt) (*UserNode, error) {
	bn, err := node.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	ds := bn.MetaStore()

	segStore, err := segment.NewSegStore(bn.Repo.FileStore())
	if err != nil {
		return nil, err
	}

	keyset, err := bn.RoleMgr.RoleGetKeyset(bn.RoleID())
	if err != nil {
		return nil, err
	}

	ids := data.New(ds, segStore, bn.NetServiceImpl)

	om := uorder.NewOrderMgr(ctx, bn.RoleID(), keyset.VerifyKey().Hash(), ds, bn.PPool, bn.RoleMgr, bn.NetServiceImpl, ids)

	ls, err := lfs.New(ctx, bn.RoleID(), keyset, ds, segStore, om)
	if err != nil {
		return nil, err
	}

	un := &UserNode{
		BaseNode:   bn,
		LfsService: ls,
		ctx:        ctx,
	}

	return un, nil
}

// start service related
func (u *UserNode) Start() error {
	go u.OpenTest()

	// register net msg handle
	u.GenericService.Register(pb.NetMessage_SayHello, u.DefaultHandler)
	u.GenericService.Register(pb.NetMessage_Get, u.HandleGet)

	u.TxMsgHandle.Register(u.BaseNode.TxMsgHandler)
	u.BlockHandle.Register(u.BaseNode.TxBlockHandler)

	u.RPCServer.Register("Memoriae", api.PermissionedUserAPI(u))

	// wait for sync
	u.PPool.Start()
	for {
		if u.PPool.Ready() {
			break
		} else {
			logger.Debug("wait for sync")
			time.Sleep(5 * time.Second)
		}
	}

	// wait for register
	err := u.Register()
	if err != nil {
		return err
	}

	// start lfs service and its ordermgr service
	u.LfsService.Start()

	logger.Info("start user for: ", u.RoleID())
	return nil
}

func (u *UserNode) RunDaemon(ready chan interface{}) error {
	return u.BaseNode.RunDaemon(ready)
}

func (u *UserNode) Close() {
	u.BaseNode.Close()
}
