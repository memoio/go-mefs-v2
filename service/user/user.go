package user

import (
	"context"
	"sync"

	"github.com/memoio/go-mefs-v2/api"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/tx"
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

	segStore segment.SegmentStore

	ctx context.Context
}

func New(ctx context.Context, opts ...node.BuilderOpt) (*UserNode, error) {
	bn, err := node.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	segStore, err := segment.NewSegStore(bn.Repo.FileStore())
	if err != nil {
		return nil, err
	}

	keyset, err := bn.RoleMgr.RoleGetKeyset()
	if err != nil {
		return nil, err
	}

	ids := data.New(bn.MetaStore(), segStore, bn.NetServiceImpl)

	om := uorder.NewOrderMgr(ctx, bn.RoleID(), keyset.VerifyKey().Hash(), bn.MetaStore(), bn.RoleMgr, bn.NetServiceImpl, ids)

	ls, err := lfs.New(ctx, bn.RoleID(), keyset, bn.MetaStore(), segStore, om)
	if err != nil {
		return nil, err
	}

	un := &UserNode{
		BaseNode:   bn,
		LfsService: ls,
		ctx:        ctx,
		segStore:   segStore,
	}

	return un, nil
}

// start service related
func (u *UserNode) Start() error {
	// register net msg handle
	u.GenericService.Register(pb.NetMessage_Get, u.defaultHandler)

	u.TxMsgHandle.Register(tx.DataTxErr, u.defaultPubsubHandler)

	u.RPCServer.Register("Memoriae", api.PermissionedUserAPI(u))

	logger.Info("start user for: ", u.RoleID())
	return nil
}

func (u *UserNode) RunDaemon(ready chan interface{}) error {
	return u.BaseNode.RunDaemon(ready)
}

func (u *UserNode) Close() {
	u.BaseNode.Close()
}
