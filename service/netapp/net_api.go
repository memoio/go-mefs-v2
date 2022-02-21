package netapp

import (
	"context"
	"math/rand"
	"time"

	"github.com/gogo/protobuf/proto"
	"golang.org/x/xerrors"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/memoio/go-mefs-v2/api"
	hs "github.com/memoio/go-mefs-v2/lib/hotstuff"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
)

var logger = logging.Logger("netApp")

// wrap net direct send and pubsub

var _ api.INetService = (*netServiceAPI)(nil)

type netServiceAPI struct {
	*NetServiceImpl
}

func (c *NetServiceImpl) SendMetaRequest(ctx context.Context, id uint64, typ pb.NetMessage_MsgType, value, sig []byte) (*pb.NetMessage, error) {
	ctx, cancle := context.WithTimeout(ctx, 3*time.Second)
	defer cancle()

	for {
		select {
		case <-ctx.Done():
			logger.Warn("found no network id for roleID: ", id)
			return nil, ctx.Err()
		default:
			c.lk.RLock()
			pid, ok := c.idMap[id]
			c.lk.RUnlock()
			if !ok {
				c.lk.RLock()
				_, has := c.wants[id]
				c.lk.RUnlock()
				if !has {
					c.lk.Lock()
					c.wants[id] = time.Now()
					c.lk.Unlock()
					c.FindPeerID(ctx, id)
				}

				time.Sleep(1 * time.Second)
			} else {
				resp, err := c.GenericService.SendNetRequest(context.TODO(), pid, c.roleID, typ, value, sig)
				if err != nil {
					c.lk.Lock()
					nt, ok := c.peerMap[pid]
					if ok {
						// remove
						if nt.Add(10 * time.Minute).Before(time.Now()) {
							delete(c.idMap, id)
							delete(c.peerMap, pid)
							c.wants[id] = time.Now()
							go c.FindPeerID(c.ctx, id)
						}
					}
					c.lk.Unlock()
					return nil, err
				}

				c.lk.Lock()
				c.peerMap[pid] = time.Now()
				c.lk.Unlock()

				return resp, nil
			}
		}
	}
}

func (c *NetServiceImpl) PublishTxMsg(ctx context.Context, msg *tx.SignedMessage) error {
	data, err := msg.Serialize()
	if err != nil {
		return err
	}

	logger.Debug("push out message: ", msg.From, msg.To, msg.Method, msg.Nonce, len(data))

	return c.msgTopic.Publish(ctx, data)
}

func (c *NetServiceImpl) PublishTxBlock(ctx context.Context, sb *tx.SignedBlock) error {
	data, err := sb.Serialize()
	if err != nil {
		return err
	}

	logger.Debug("push out block: ", sb.GroupID, sb.MinerID, sb.Height, sb.Slot, len(data))

	return c.blockTopic.Publish(ctx, data)
}

func (c *NetServiceImpl) PublishHsMsg(ctx context.Context, hm *hs.HotstuffMessage) error {
	data, err := hm.Serialize()
	if err != nil {
		return err
	}

	logger.Debug("push out ht msg: ", hm.From, hm.Type, len(data))

	return c.hsTopic.Publish(ctx, data)
}

func (c *NetServiceImpl) PublishEvent(ctx context.Context, em *pb.EventMessage) error {
	data, err := proto.Marshal(em)
	if err != nil {
		return err
	}

	logger.Debug("push out event msg: ", em.Type, len(data))

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
	if c.lastFetch.Validate() == nil {
		resp, err := c.GenericService.SendNetRequest(ctx, c.lastFetch, c.roleID, pb.NetMessage_Get, key, nil)
		if err == nil && resp.GetHeader().GetType() != pb.NetMessage_Err {
			logger.Debug("receive data from last good: ", c.lastFetch.Pretty(), string(key))
			return resp.GetData().GetMsgInfo(), nil
		}
	}

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

		c.lastFetch = pi.ID

		return resp.GetData().GetMsgInfo(), nil
	}

	return nil, xerrors.Errorf("fetch %s time out", string(key))
}
