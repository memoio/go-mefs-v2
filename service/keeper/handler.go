package keeper

import (
	"context"
	"log"

	peer "github.com/libp2p/go-libp2p-core/peer"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
)

func (k *KeeperNode) defaultHandler(ctx context.Context, p peer.ID, mes *pb.NetMessage) (*pb.NetMessage, error) {
	mes.Data.MsgInfo = []byte("hello keeper")
	return mes, nil
}

func (k *KeeperNode) defaultPubsubHandler(ctx context.Context, mes *tx.SignedMessage) error {
	log.Println("keeper received pub msg:", mes.Method, mes.From)
	return nil
}
