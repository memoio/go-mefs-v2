package core_service

import (
	"context"
	"fmt"

	"github.com/gogo/protobuf/proto"
	lru "github.com/hashicorp/golang-lru"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/routing"
	"github.com/pkg/errors"

	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/service"
	generic_service "github.com/memoio/go-mefs-v2/service/core/generic"
	"github.com/memoio/go-mefs-v2/service/core/instance"
	"github.com/memoio/go-mefs-v2/submodule/network"
)

var _ service.CoreService = (*CoreServiceImpl)(nil)

type CoreServiceImpl struct {
	*generic_service.GenericService
	// MetaDs   repo.Datastore
	accounts *lru.ARCCache // nodeid->netid
	rt       routing.Routing
	h        host.Host
}

func New(ctx context.Context, ns *network.NetworkSubmodule, s instance.Subscriber) (*CoreServiceImpl, error) {
	if s == nil {
		s = instance.New()
	}

	service, err := generic_service.New(ctx, ns, s)
	if err != nil {
		return nil, err
	}

	accounts, err := lru.NewARC(2048)
	if err != nil {
		return nil, err
	}

	core := &CoreServiceImpl{}
	core.GenericService = service
	core.rt = ns.Router
	core.h = ns.Host
	core.accounts = accounts
	return core, nil
}

func (c *CoreServiceImpl) Host() host.Host {
	return c.h
}

func (c *CoreServiceImpl) Routing() routing.Routing {
	return c.rt
}

func (c *CoreServiceImpl) Info(ctx context.Context, to string) (*pb.NetInfo, error) {
	val, ok := c.accounts.Get(to)
	if !ok {
		return nil, errors.New("not found")
	}
	return val.(*pb.NetInfo), nil
}

func (c *CoreServiceImpl) NetAddr(ctx context.Context, to string) (peer.AddrInfo, error) {
	val, ok := c.accounts.Get(to)
	if !ok {
		return peer.AddrInfo{}, errors.New("not found")
	}
	info := val.(*pb.NetInfo)

	return peer.AddrInfo{
		ID: peer.ID(info.NetID),
	}, nil
}

func (c *CoreServiceImpl) GetNetInfo(ctx context.Context, to peer.ID) (*pb.NetInfo, error) {
	fmt.Println("CoreServiceImpl.GetInfo")

	resp, err := c.SendMetaRequest(ctx, to, pb.NetMessage_Get, []byte("Role"))
	if err != nil {
		fmt.Println("GetInfo,err 2: ", err)
		return nil, err
	}
	res := &pb.NetInfo{}

	err = proto.Unmarshal(resp.GetData().GetMsgInfo(), res)
	if err != nil {
		fmt.Println("GetInfo,err 3: ", err)
		return nil, err
	}
	return res, nil
}
