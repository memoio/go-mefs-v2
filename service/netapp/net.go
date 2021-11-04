package netapp

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/routing"
	pubsub "github.com/libp2p/go-libp2p-pubsub"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/build"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/service/netapp/generic"
	"github.com/memoio/go-mefs-v2/service/netapp/handler"
	"github.com/memoio/go-mefs-v2/submodule/network"
)

var logger = logging.Logger("NetApp")

var _ api.INetService = (*NetServiceImpl)(nil)

var ErrTimeOut = errors.New("time out")

// wrap net interface
type NetServiceImpl struct {
	sync.RWMutex
	*generic.GenericService
	handler.TxMsgHandle // handle pubsub tx msg
	handler.EventHandle // handle pubsub event msg

	ctx    context.Context
	roleID uint64  // local node id
	netID  peer.ID // local net id

	ds store.KVStore

	idMap map[uint64]peer.ID
	wants map[uint64]time.Time

	rt routing.Routing
	h  host.Host

	eventTopic *pubsub.Topic // used to find peerID depends on roleID
	msgTopic   *pubsub.Topic

	related []uint64
}

func New(ctx context.Context, roleID uint64, ds store.KVStore, ns *network.NetworkSubmodule) (*NetServiceImpl, error) {
	ph := handler.NewTxMsgHandle()
	peh := handler.NewEventHandle()

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

	core := &NetServiceImpl{
		GenericService: gs,
		TxMsgHandle:    ph,
		EventHandle:    peh,
		ctx:            ctx,
		roleID:         roleID,
		netID:          ns.NetID(ctx),
		ds:             ds,
		rt:             ns.Router,
		h:              ns.Host,
		idMap:          make(map[uint64]peer.ID),
		wants:          make(map[uint64]time.Time),
		related:        make([]uint64, 0, 128),
		eventTopic:     eTopic,
		msgTopic:       mTopic,
	}

	// register for find peer
	peh.Register(pb.EventMessage_GetPeer, core.handleGetPeer)
	peh.Register(pb.EventMessage_PutPeer, core.handlePutPeer)

	go core.handleIncomingEvent(ctx)
	go core.handleIncomingMessage(ctx)

	go core.regularPeerFind(ctx)

	return core, nil
}

func (c *NetServiceImpl) RoleID() uint64 {
	return c.roleID
}

// add a new node
func (c *NetServiceImpl) AddNode(id uint64) {
	c.Lock()
	has := false
	for _, uid := range c.related {
		if uid == id {
			has = true
		}
	}

	if !has {
		c.related = append(c.related, id)
	}

	_, ok := c.idMap[id]
	if !ok {
		_, ok = c.wants[id]
		if !ok {
			c.wants[id] = time.Now()
			c.FindPeerID(c.ctx, id)
		}
	}
	c.Unlock()
}

func (c *NetServiceImpl) SendMetaMessage(ctx context.Context, id uint64, typ pb.NetMessage_MsgType, value []byte) error {
	pid, ok := c.idMap[id]
	if ok {
		return c.GenericService.SendMetaMessage(ctx, pid, typ, value)
	}
	return nil
}

func (c *NetServiceImpl) SendMetaRequest(ctx context.Context, id uint64, typ pb.NetMessage_MsgType, value []byte) (*pb.NetMessage, error) {
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
				return c.GenericService.SendMetaRequest(ctx, pid, typ, value)
			}
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
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.Lock()
			for id, t := range c.wants {
				nt := time.Now()
				if t.Add(30 * time.Second).Before(nt) {
					c.FindPeerID(ctx, id)
					c.wants[id] = nt
				}
			}
			c.Unlock()
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
					fmt.Println("handle event message: ", em.Type)
					c.EventHandle.HandleMessage(ctx, em)
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
				err := sm.Deserilize(received.GetData())
				if err == nil {
					c.TxMsgHandle.HandleMessage(ctx, sm)
				}
			}
		}
	}()
}

func (c *NetServiceImpl) PublishMsg(ctx context.Context, msg *tx.SignedMessage) error {
	data, err := msg.Serialize()
	if err != nil {
		return err
	}
	return c.msgTopic.Publish(ctx, data)
}

func (c *NetServiceImpl) PublishEvent(ctx context.Context, msg *pb.EventMessage) error {
	data, _ := proto.Marshal(msg)
	return c.eventTopic.Publish(ctx, data)
}

// fetch
func (c *NetServiceImpl) FetchMsg(ctx context.Context, msgID []byte) error {
	// iter over connected peers
	return nil
}

func (c *NetServiceImpl) FetchBlock(ctx context.Context, msgID []byte) error {
	// iter over connected peers
	return nil
}
