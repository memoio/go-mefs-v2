package api

import (
	"context"

	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/libp2p/go-libp2p-core/metrics"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"

	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
)

type FullNode interface {
	IAuth
	IConfig
	IWallet
}

type IAuth interface {
	AuthVerify(context.Context, string) ([]auth.Permission, error)
	AuthNew(context.Context, []auth.Permission) ([]byte, error)
}

type IConfig interface {
	ConfigSet(context.Context, string, string) error
	ConfigGet(context.Context, string) (interface{}, error)
}

type IWallet interface {
	WalletNew(context.Context, types.KeyType) (address.Address, error)
	WalletSign(context.Context, address.Address, []byte) ([]byte, error)
	WalletList(context.Context) ([]address.Address, error)
	WalletHas(context.Context, address.Address) (bool, error)
	WalletDelete(context.Context, address.Address) error
	WalletExport(context.Context, address.Address) (*types.KeyInfo, error)
	WalletImport(context.Context, *types.KeyInfo) (address.Address, error)
}

type INetwork interface {
	// info
	NetAddrInfo(context.Context) (peer.AddrInfo, error)

	NetConnectedness(context.Context, peer.ID) (network.Connectedness, error)

	NetConnect(context.Context, peer.AddrInfo) error

	NetDisconnect(context.Context, peer.ID) error

	NetFindPeer(context.Context, peer.ID) (peer.AddrInfo, error)

	NetPeerInfo(context.Context, peer.ID) (*ExtendedPeerInfo, error)

	NetPeers(context.Context) ([]peer.AddrInfo, error)

	// status; add more
	NetAutoNatStatus(context.Context) (NatInfo, error)
	NetBandwidthStats(ctx context.Context) (metrics.Stats, error)
	NetBandwidthStatsByPeer(ctx context.Context) (map[string]metrics.Stats, error)
	NetBandwidthStatsByProtocol(ctx context.Context) (map[protocol.ID]metrics.Stats, error)
}

type IRole interface {
	RoleSelf() (pb.RoleInfo, error)
	RoleGet(uint64) (pb.RoleInfo, error)
}

type INetService interface {
	// send/handle msg directly over network
	SendMetaMessage(ctx context.Context, to uint64, mes_typ pb.NetMessage_MsgType, val []byte) error
	SendMetaRequest(ctx context.Context, to uint64, mes_typ pb.NetMessage_MsgType, val []byte) (*pb.NetMessage, error)

	PublishTxMsg(ctx context.Context, msg *tx.SignedMessage) error
	PublishTxBlock(ctx context.Context, msg *tx.Block) error
	PublishEvent(ctx context.Context, msg *pb.EventMessage) error
}
