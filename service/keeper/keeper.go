package keeper

import (
	"context"
	"strconv"
	"sync"

	"github.com/memoio/go-mefs-v2/lib/pb"
	core_service "github.com/memoio/go-mefs-v2/service/core"
	"github.com/memoio/go-mefs-v2/service/core/instance"
	"github.com/memoio/go-mefs-v2/submodule/node"
	"github.com/pkg/errors"
)

var _ instance.Subscriber = (*KeeperNode)(nil)

type KeeperNode struct {
	sync.RWMutex

	*node.BaseNode

	// handle
	*instance.Impl

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
		Impl:     instance.New(),
	}

	return kn, nil
}

// start service related
func (k *KeeperNode) Start() error {
	id, err := strconv.Atoi(k.Repo.Config().Identity.Name)
	if err != nil {
		return err
	}

	// register handles
	cs, err := core_service.New(k.ctx, uint64(id), k.Repo.MetaStore(), k.NetworkSubmodule, k)
	if err != nil {
		return errors.Wrap(err, "failed to create core service")
	}

	k.BaseNode.CoreServiceImpl = cs

	k.Register(pb.NetMessage_Get, k.defaultHandler)
	return nil
}

func (k *KeeperNode) RunDaemon(ready chan interface{}) error {
	return k.BaseNode.RunDaemon(ready)
}

func (k *KeeperNode) Close() {

}
