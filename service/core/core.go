package core_service

import (
	"context"
	"fmt"
	"log"

	"github.com/gogo/protobuf/proto"
	lru "github.com/hashicorp/golang-lru"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/routing"
	swarm "github.com/libp2p/go-libp2p-swarm"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"

	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/service"
	generic_service "github.com/memoio/go-mefs-v2/service/core/generic"
	"github.com/memoio/go-mefs-v2/service/core/instance"
)

var _ service.CoreService = (*CoreServiceImpl)(nil)

type CoreServiceImpl struct {
	*generic_service.GenericService
	// MetaDs   repo.Datastore
	accounts *lru.ARCCache
	rt       routing.Routing
	h        host.Host
}

func New(ctx context.Context, h host.Host, rt routing.Routing, s instance.Subscriber) (*CoreServiceImpl, error) {
	if s == nil {
		s = instance.New()
	}

	service, err := generic_service.New(ctx, h, s)
	if err != nil {
		return nil, err
	}

	accounts, err := lru.NewARC(2048)
	if err != nil {
		return nil, err
	}

	core := &CoreServiceImpl{}
	core.GenericService = service
	core.rt = rt
	core.h = h
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

// 查看所有连接的节点
func (c *CoreServiceImpl) Peers(ctx context.Context) ([]peer.AddrInfo, error) {
	conns := c.h.Network().Conns()
	out := make([]peer.AddrInfo, len(conns))

	for i, conn := range conns {
		out[i] = peer.AddrInfo{
			ID: conn.RemotePeer(),
			Addrs: []ma.Multiaddr{
				conn.RemoteMultiaddr(),
			},
		}
	}
	return out, nil
}

// 查看所有记录的节点
func (c *CoreServiceImpl) PeersInfo(ctx context.Context) ([]*pb.NetInfo, error) {
	keys := c.accounts.Keys()
	out := make([]*pb.NetInfo, len(keys))
	for i, key := range keys {
		val, ok := c.accounts.Get(key)
		if !ok {
			return nil, errors.New("not found")
		}
		info := val.(*pb.NetInfo)
		out[i] = info
	}
	return out, nil
}

func (c *CoreServiceImpl) Connect(ctx context.Context, addr string) error {
	swrm, ok := c.h.Network().(*swarm.Swarm)
	if !ok {
		return fmt.Errorf("peerhost network was not a swarm")
	}

	a, err := ma.NewMultiaddr(addr)
	if err != nil {
		return err
	}
	pinfo, err := peer.AddrInfoFromP2pAddr(a)
	if err != nil {
		return err
	}
	swrm.Backoff().Clear(pinfo.ID)
	err = c.h.Connect(ctx, *pinfo)
	return err
}

func (c *CoreServiceImpl) TryConnect(ctx context.Context, to peer.ID) (string, bool) {
	if c.h == nil || c.rt == nil {
		return "", false
	}

	if c.h.Network().Connectedness(to) == network.Connected {
		return "", true
	}

	connectTryCount := 1
	for i := 0; i < connectTryCount; i++ {
		pi, err := c.rt.FindPeer(ctx, to)
		if err != nil {
			break
		}

		for j := 0; j < 3; j++ {
			if swrm, ok := c.h.Network().(*swarm.Swarm); ok {
				swrm.Backoff().Clear(pi.ID)
			}

			err = c.h.Connect(ctx, pi)
			if err == nil {
				if c.h.Network().Connectedness(to) == network.Connected {
					//get external address
					mAddr, err := peer.AddrInfoToP2pAddrs(&pi)
					if err != nil || len(mAddr) < 1 {
						log.Println("AddrInfo ", pi, " to p2pAddrs err: ", err)
						return "", true
					}
					return mAddr[0].String(), true
				}
			}
		}
	}

	return "", false
}

func (c *CoreServiceImpl) Disconnect(ctx context.Context, to peer.ID) error {
	return c.h.Network().ClosePeer(to)
}

func (c *CoreServiceImpl) GetNetInfo(ctx context.Context, to peer.ID) (*pb.NetInfo, error) {
	fmt.Println("CoreServiceImpl.GetInfo")

	data, err := c.SendMetaRequest(ctx, to, pb.NetMessage_Get, "Role", nil)
	if err != nil {
		fmt.Println("GetInfo,err 2: ", err)
		return nil, err
	}
	res := &pb.NetInfo{}

	fmt.Println("CoreServiceImpl.GetInfo, len(data): ", len(data))

	err = proto.Unmarshal(data, res)
	if err != nil {
		fmt.Println("GetInfo,err 3: ", err)
		return nil, err
	}
	return res, nil
}
