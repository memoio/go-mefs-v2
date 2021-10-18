package types

import (
	"context"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	mpb "github.com/memoio/go-mefs-v2/lib/pb"
)

// based on p2p?
type BasicNetwork interface {
	Connectedness(context.Context, peer.ID)
	Connect(context.Context, peer.AddrInfo)
	DisConnect(context.Context, peer.ID)
	List(context.Context) ([]peer.AddrInfo, error)
	FindPeer(context.Context, peer.ID) (peer.AddrInfo, error)
	ListenAddr(context.Context) (peer.AddrInfo, error)
}

type NetInfo struct {
	netID     string
	endpoint  []peer.AddrInfo // from mpb.NetInfo
	credit    int32           // 评分
	connected bool            // 是否连接
	pin       bool            // 是否和本节点相关，相关的话，尽量保持连接，以及持久化保存
	banned    bool            // 是否被禁
	lastSeen  time.Time
}

type NetMgr struct {
	bootStrappers []*peer.AddrInfo    // 启动时候连接的节点
	nodes         map[string]*NetInfo // key: netID
	blockList     map[string]uint64   // key: ip addr; value: block time

	network BasicNetwork // 具体网络操作
}

type NetApp interface {
	// 发送给MID
	SendMessage(context.Context, uint64, *mpb.NetMessage) error
	SendMessageWithResponse(context.Context, uint64, *mpb.NetMessage) (*mpb.NetMessage, error)
	// 是否需要
	Publish(context.Context, []byte)
}
