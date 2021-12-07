package netapp

import (
	"context"
	"math/rand"
	"time"

	"github.com/gogo/protobuf/proto"
	"golang.org/x/xerrors"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/memoio/go-mefs-v2/api"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
)

var logger = logging.Logger("netApp")

var ErrTimeOut = xerrors.New("send time out")

// wrap net direct send and pubsub

var _ api.INetService = (*netServiceAPI)(nil)

type netServiceAPI struct {
	*NetServiceImpl
}

func (c *NetServiceImpl) SendMetaRequest(ctx context.Context, id uint64, typ pb.NetMessage_MsgType, value, sig []byte) (*pb.NetMessage, error) {
	ctx, cancle := context.WithTimeout(ctx, 30*time.Second)
	defer cancle()

	for {
		select {
		case <-ctx.Done():
			return nil, ErrTimeOut
		default:
			c.RLock()
			pid, ok := c.idMap[id]
			c.RUnlock()
			if !ok {
				c.RLock()
				_, has := c.wants[id]
				c.RUnlock()
				if !has {
					c.Lock()
					c.wants[id] = time.Now()
					c.Unlock()
					c.FindPeerID(ctx, id)
				}

				time.Sleep(1 * time.Second)
			} else {
				return c.GenericService.SendNetRequest(ctx, pid, c.roleID, typ, value, sig)
			}
		}

	}
}

func (c *NetServiceImpl) PublishTxMsg(ctx context.Context, msg *tx.SignedMessage) error {
	data, err := msg.Serialize()
	if err != nil {
		return err
	}
	return c.msgTopic.Publish(ctx, data)
}

func (c *NetServiceImpl) PublishTxBlock(ctx context.Context, msg *tx.Block) error {
	data, _ := msg.Serialize()
	return c.blockTopic.Publish(ctx, data)
}

func (c *NetServiceImpl) PublishEvent(ctx context.Context, msg *pb.EventMessage) error {
	data, _ := proto.Marshal(msg)
	return c.eventTopic.Publish(ctx, data)
}

func disorder(array []peer.AddrInfo) {
	var temp peer.AddrInfo
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := len(array) - 1; i >= 0; i-- {
		num := r.Intn(i + 1)
		temp = array[i]
		array[i] = array[num]
		array[num] = temp
	}
}

// fetch
func (c *NetServiceImpl) Fetch(ctx context.Context, key []byte) ([]byte, error) {
	// iter over connected peers
	pinfos, err := c.ns.NetPeers(ctx)
	if err != nil {
		return nil, err
	}

	disorder(pinfos)

	for _, pi := range pinfos {
		resp, err := c.GenericService.SendNetRequest(ctx, pi.ID, c.roleID, pb.NetMessage_Get, key, nil)
		if err != nil {
			continue
		}

		if resp.GetHeader().GetType() == pb.NetMessage_Err {
			continue
		}

		return resp.GetData().GetMsgInfo(), nil
	}

	return nil, ErrTimeOut
}
