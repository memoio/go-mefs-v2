package netapp

import (
	"context"
	"encoding/binary"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"go.opencensus.io/tag"

	"github.com/memoio/go-mefs-v2/build"
	hs "github.com/memoio/go-mefs-v2/lib/hotstuff"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/service/netapp/generic"
	"github.com/memoio/go-mefs-v2/service/netapp/handler"
	"github.com/memoio/go-mefs-v2/submodule/metrics"
	"github.com/memoio/go-mefs-v2/submodule/network"
)

// wrap net interface
type NetServiceImpl struct {
	sync.RWMutex
	*generic.GenericService
	handler.TxMsgHandle // handle pubsub tx msg
	handler.EventHandle // handle pubsub event msg
	handler.BlockHandle
	handler.HsMsgHandle

	ctx    context.Context
	roleID uint64  // local node id
	netID  peer.ID // local net id

	ds store.KVStore

	lastFetch peer.ID

	idMap map[uint64]peer.ID
	wants map[uint64]time.Time

	peers map[peer.ID]struct{}

	ns *network.NetworkSubmodule

	eventTopic *pubsub.Topic // used to find peerID depends on roleID
	msgTopic   *pubsub.Topic
	blockTopic *pubsub.Topic
	hsTopic    *pubsub.Topic

	related []uint64
}

func New(ctx context.Context, roleID uint64, ds store.KVStore, ns *network.NetworkSubmodule) (*NetServiceImpl, error) {
	ph := handler.NewTxMsgHandle()
	peh := handler.NewEventHandle()
	bh := handler.NewBlockHandle()
	hh := handler.NewHsMsgHandle()

	gs, err := generic.New(ctx, ns)
	if err != nil {
		return nil, err
	}

	eTopic, err := ns.Pubsub.Join(build.EventTopic(ns.NetworkName))
	if err != nil {
		return nil, err
	}

	mTopic, err := ns.Pubsub.Join(build.MsgTopic(ns.NetworkName))
	if err != nil {
		return nil, err
	}

	bTopic, err := ns.Pubsub.Join(build.BlockTopic(ns.NetworkName))
	if err != nil {
		return nil, err
	}

	hTopic, err := ns.Pubsub.Join(build.HSMsgTopic(ns.NetworkName))
	if err != nil {
		return nil, err
	}

	core := &NetServiceImpl{
		GenericService: gs,
		TxMsgHandle:    ph,
		EventHandle:    peh,
		BlockHandle:    bh,
		HsMsgHandle:    hh,
		roleID:         roleID,
		netID:          ns.NetID(ctx),
		ds:             ds,
		ns:             ns,
		idMap:          make(map[uint64]peer.ID),
		wants:          make(map[uint64]time.Time),
		peers:          make(map[peer.ID]struct{}),
		related:        make([]uint64, 0, 128),
		eventTopic:     eTopic,
		msgTopic:       mTopic,
		blockTopic:     bTopic,
		hsTopic:        hTopic,
	}

	ctx, _ = tag.New(
		ctx,
		tag.Upsert(metrics.NetPeerID, ns.NetID(ctx).Pretty()),
	)

	core.ctx = ctx

	// register for find peer
	peh.Register(pb.EventMessage_GetPeer, core.handleGetPeer)
	peh.Register(pb.EventMessage_PutPeer, core.handlePutPeer)

	go core.handleIncomingEvent(ctx)
	go core.handleIncomingMessage(ctx)
	go core.handleIncomingBlock(ctx)
	go core.handleIncomingHSMsg(ctx)

	go core.regularPeerFind(ctx)

	return core, nil
}

func (c *NetServiceImpl) API() *netServiceAPI {
	return &netServiceAPI{c}
}

// add a new node
func (c *NetServiceImpl) AddNode(id uint64, pid peer.ID) {
	c.Lock()
	defer c.Unlock()

	has := false
	for _, uid := range c.related {
		if uid == id {
			has = true
		}
	}

	if !has {
		c.related = append(c.related, id)
	}

	if pid.Validate() == nil {
		c.idMap[id] = pid
		return
	}

	_, ok := c.idMap[id]
	if !ok {
		_, ok = c.wants[id]
		if !ok {
			c.wants[id] = time.Now()
			c.FindPeerID(c.ctx, id)
		}
	}

}

func (c *NetServiceImpl) FindPeerID(ctx context.Context, id uint64) error {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, id)

	em := &pb.EventMessage{
		Type: pb.EventMessage_GetPeer,
		Data: buf,
	}

	return c.PublishEvent(ctx, em)
}

func (c *NetServiceImpl) regularPeerFind(ctx context.Context) {
	ticker := time.NewTicker(300 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.Lock()
			for id, t := range c.wants {
				_, ok := c.idMap[id]
				if ok {
					continue
				}
				nt := time.Now()
				if t.Add(30 * time.Second).Before(nt) {
					c.FindPeerID(ctx, id)
					c.wants[id] = nt
				}
			}
			c.Unlock()

			pinfos, err := c.ns.NetPeers(ctx)
			if err != nil {
				continue
			}

			for _, pi := range pinfos {
				resp, err := c.GenericService.SendNetRequest(ctx, pi.ID, c.roleID, pb.NetMessage_SayHello, nil, nil)
				if err != nil {
					continue
				}

				if resp.GetHeader().GetType() == pb.NetMessage_Err {
					continue
				}

				rid := binary.BigEndian.Uint64(resp.GetData().GetMsgInfo())

				c.idMap[rid] = pi.ID
			}

		case <-c.ctx.Done():
			return
		}
	}
}

// handle event msg
func (c *NetServiceImpl) handleIncomingEvent(ctx context.Context) {
	sub, err := c.eventTopic.Subscribe()
	if err != nil {
		return
	}

	go func() {
		for {
			received, err := sub.Next(ctx)
			if err != nil {
				return
			}
			from := received.GetFrom()

			if c.netID != from {
				em := new(pb.EventMessage)
				err := proto.Unmarshal(received.GetData(), em)
				if err == nil {
					logger.Debug("handle event message: ", em.Type)
					c.EventHandle.Handle(ctx, em)
				}
			}
		}
	}()
}

func (c *NetServiceImpl) handleGetPeer(ctx context.Context, mes *pb.EventMessage) error {
	id := binary.BigEndian.Uint64(mes.GetData())
	if id == c.roleID {
		logger.Debug(c.netID.Pretty(), "handle find peer of role:", id)
		pi := &pb.PutPeerInfo{
			RoleID: c.roleID,
			NetID:  []byte(c.netID),
		}

		pdata, _ := proto.Marshal(pi)
		em := &pb.EventMessage{
			Type: pb.EventMessage_PutPeer,
			Data: pdata,
		}

		return c.PublishEvent(ctx, em)
	}

	return nil
}

func (c *NetServiceImpl) handlePutPeer(ctx context.Context, mes *pb.EventMessage) error {
	ppi := new(pb.PutPeerInfo)
	err := proto.Unmarshal(mes.GetData(), ppi)
	if err != nil {
		return err
	}

	netID, err := peer.IDFromBytes(ppi.NetID)
	if err != nil {
		return err
	}

	logger.Debug(c.netID.Pretty(), "handle put peer:", netID.Pretty())

	c.Lock()
	_, ok := c.wants[ppi.GetRoleID()]
	if ok {
		c.idMap[ppi.RoleID] = netID
		delete(c.wants, ppi.RoleID)
	}
	c.Unlock()
	return nil
}

// handle net message
func (c *NetServiceImpl) handleIncomingMessage(ctx context.Context) {
	sub, err := c.msgTopic.Subscribe()
	if err != nil {
		return
	}

	go func() {
		for {
			received, err := sub.Next(ctx)
			if err != nil {
				return
			}

			from := received.GetFrom()

			if c.netID != from {
				// handle it
				// umarshal pmsg data
				sm := new(tx.SignedMessage)
				err := sm.Deserialize(received.GetData())
				if err == nil {
					c.TxMsgHandle.Handle(ctx, sm)
				}
			}
		}
	}()
}

func (c *NetServiceImpl) handleIncomingBlock(ctx context.Context) {
	sub, err := c.blockTopic.Subscribe()
	if err != nil {
		return
	}

	go func() {
		for {
			received, err := sub.Next(ctx)
			if err != nil {
				return
			}

			from := received.GetFrom()

			if c.netID != from {
				// handle it
				// umarshal pmsg data
				sm := new(tx.SignedBlock)
				err := sm.Deserialize(received.GetData())
				if err == nil {
					c.BlockHandle.Handle(ctx, sm)
				}
			}
		}
	}()
}

func (c *NetServiceImpl) handleIncomingHSMsg(ctx context.Context) {
	sub, err := c.hsTopic.Subscribe()
	if err != nil {
		return
	}

	go func() {
		for {
			received, err := sub.Next(ctx)
			if err != nil {
				return
			}

			from := received.GetFrom()

			if c.netID != from {
				// handle it
				// umarshal pmsg data
				sm := new(hs.HotstuffMessage)
				err := sm.Deserialize(received.GetData())
				if err == nil {
					c.HsMsgHandle.Handle(ctx, sm)
				}
			}
		}
	}()
}
