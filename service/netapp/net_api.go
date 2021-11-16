package netapp

import (
	"context"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
)

func (c *NetServiceImpl) SendMetaMessage(ctx context.Context, id uint64, typ pb.NetMessage_MsgType, value []byte) error {
	pid, ok := c.idMap[id]
	if ok {
		return c.GenericService.SendNetMessage(ctx, pid, typ, value)
	}
	return nil
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
				return c.GenericService.SendNetRequest(ctx, pid, c.RoleID(), typ, value, sig)
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

// fetch
func (c *NetServiceImpl) Fetch(ctx context.Context, key []byte) ([]byte, error) {
	// iter over connected peers
	pinfos, err := c.ns.NetPeers(ctx)
	if err != nil {
		return nil, err
	}

	for _, pi := range pinfos {
		resp, err := c.GenericService.SendNetRequest(ctx, pi.ID, c.RoleID(), pb.NetMessage_Get, key, nil)
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

func (c *NetServiceImpl) FetchBlock(ctx context.Context, msgID []byte) error {
	// iter over connected peers
	return nil
}
