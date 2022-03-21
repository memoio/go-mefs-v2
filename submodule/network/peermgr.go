package network

import (
	"context"
	"strings"
	"sync"
	"time"

	"go.opencensus.io/stats"
	"golang.org/x/xerrors"

	"github.com/libp2p/go-libp2p-core/event"
	host "github.com/libp2p/go-libp2p-core/host"
	net "github.com/libp2p/go-libp2p-core/network"
	peer "github.com/libp2p/go-libp2p-core/peer"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/memoio/go-mefs-v2/submodule/metrics"
)

const (
	MaxPeers = 320
	MinPeers = 128
)

type IPeerMgr interface {
	AddPeer(p peer.ID)
	GetPeerLatency(p peer.ID) (time.Duration, bool)
	SetPeerLatency(p peer.ID, latency time.Duration)
	Disconnect(p peer.ID)
	Stop(ctx context.Context) error
	Run(ctx context.Context)
}

var _ IPeerMgr = &PeerMgr{}

type PeerMgr struct {
	netName       string
	bootstrappers []peer.AddrInfo

	peersLk sync.Mutex
	peers   map[peer.ID]time.Duration

	maxPeers int
	minPeers int

	expanding chan struct{}

	h   host.Host
	dht *dht.IpfsDHT

	notifee     *net.NotifyBundle
	peerEmitter event.Emitter

	done chan struct{}
}

type NewPeer struct {
	Id peer.ID //nolint
}

func NewPeerMgr(netName string, h host.Host, dht *dht.IpfsDHT, bootstrap []peer.AddrInfo) (*PeerMgr, error) {
	pm := &PeerMgr{
		netName:       netName,
		h:             h,
		dht:           dht,
		bootstrappers: bootstrap,

		peers:     make(map[peer.ID]time.Duration),
		expanding: make(chan struct{}, 1),

		maxPeers: MaxPeers,
		minPeers: MinPeers,

		done: make(chan struct{}),
	}

	emitter, err := h.EventBus().Emitter(new(NewPeer))
	if err != nil {
		return nil, xerrors.Errorf("creating NewPeer emitter: %w", err)
	}
	pm.peerEmitter = emitter

	pm.notifee = &net.NotifyBundle{
		DisconnectedF: func(_ net.Network, c net.Conn) {
			pm.Disconnect(c.RemotePeer())
		},
	}

	h.Network().Notify(pm.notifee)

	return pm, nil
}

func (pmgr *PeerMgr) AddPeer(p peer.ID) {
	_ = pmgr.peerEmitter.Emit(NewPeer{Id: p}) //nolint:errcheck
	pmgr.peersLk.Lock()
	defer pmgr.peersLk.Unlock()
	pmgr.peers[p] = time.Duration(0)
}

func (pmgr *PeerMgr) GetPeerLatency(p peer.ID) (time.Duration, bool) {
	pmgr.peersLk.Lock()
	defer pmgr.peersLk.Unlock()
	dur, ok := pmgr.peers[p]
	return dur, ok
}

func (pmgr *PeerMgr) SetPeerLatency(p peer.ID, latency time.Duration) {
	pmgr.peersLk.Lock()
	defer pmgr.peersLk.Unlock()
	if _, ok := pmgr.peers[p]; ok {
		pmgr.peers[p] = latency
	}

}

func (pmgr *PeerMgr) Disconnect(p peer.ID) {
	if pmgr.h.Network().Connectedness(p) == net.NotConnected {
		pmgr.peersLk.Lock()
		defer pmgr.peersLk.Unlock()
		delete(pmgr.peers, p)
	}
}

func (pmgr *PeerMgr) Stop(ctx context.Context) error {
	logger.Warn("closing peermgr done")
	_ = pmgr.peerEmitter.Close()
	close(pmgr.done)
	return nil
}

func (pmgr *PeerMgr) Run(ctx context.Context) {
	tick := time.NewTicker(time.Second * 60)
	defer tick.Stop()

	ltick := time.NewTicker(time.Second * 600)
	defer ltick.Stop()

	for {
		select {
		case <-tick.C:
			pcount := pmgr.getPeerCount()
			if pcount < pmgr.minPeers {
				pmgr.expandPeers()
			} else if pcount > pmgr.maxPeers {
				logger.Debugf("peer count about threshold: %d > %d", pcount, pmgr.maxPeers)
			}
			stats.Record(ctx, metrics.PeerCount.M(int64(pmgr.getPeerCount())))
		case <-ltick.C:
			if len(pmgr.bootstrappers) == 0 {
				//logger.Warn("no peers connected, and no bootstrappers configured")
				return
			}

			//logger.Info("connecting to bootstrap peers")
			for _, bsp := range pmgr.bootstrappers {
				ctx, cancel := context.WithTimeout(ctx, time.Second*3)
				err := pmgr.h.Connect(ctx, bsp)
				cancel()
				if err != nil {
					continue
				}
				protos, err := pmgr.h.Peerstore().GetProtocols(bsp.ID)
				if err != nil {
					continue
				}

				has := false
				for _, pro := range protos {
					if strings.Contains(pro, pmgr.netName) {
						has = true
						break
					}
				}

				if !has {
					pmgr.h.Network().ClosePeer(bsp.ID)
				}

			}
		case <-pmgr.done:
			logger.Warn("exiting peermgr run")
			return
		}
	}
}

func (pmgr *PeerMgr) getPeerCount() int {
	pmgr.peersLk.Lock()
	defer pmgr.peersLk.Unlock()
	return len(pmgr.peers)
}

func (pmgr *PeerMgr) expandPeers() {
	select {
	case pmgr.expanding <- struct{}{}:
	default:
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.TODO(), time.Second*30)
		defer cancel()

		pmgr.doExpand(ctx)

		<-pmgr.expanding
	}()
}

func (pmgr *PeerMgr) doExpand(ctx context.Context) {
	pcount := pmgr.getPeerCount()
	if pcount == 0 {
		if len(pmgr.bootstrappers) == 0 {
			//logger.Warn("no peers connected, and no bootstrappers configured")
			return
		}

		//logger.Info("connecting to bootstrap peers")
		for _, bsp := range pmgr.bootstrappers {
			ctx, cancel := context.WithTimeout(ctx, time.Second*3)
			err := pmgr.h.Connect(ctx, bsp)
			cancel()
			if err != nil {
				continue
			}
			protos, err := pmgr.h.Peerstore().GetProtocols(bsp.ID)
			if err != nil {
				continue
			}

			has := false
			for _, pro := range protos {
				if strings.Contains(pro, pmgr.netName) {
					has = true
					break
				}
			}

			if !has {
				pmgr.h.Network().ClosePeer(bsp.ID)
			}
		}
		return
	}

	err := pmgr.dht.Bootstrap(ctx)
	if err != nil {
		logger.Warnf("dht bootstrapping failed: %s", err)
	}
}
