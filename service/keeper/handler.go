package keeper

import (
	"context"

	peer "github.com/libp2p/go-libp2p-core/peer"
	"github.com/memoio/go-mefs-v2/lib/pb"
)

func (k *KeeperNode) defaultHandler(ctx context.Context, p peer.ID, mes *pb.NetMessage) (*pb.NetMessage, error) {
	mes.Data.MsgInfo = []byte("hello keeper")
	return mes, nil
}
