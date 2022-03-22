package internal

import (
	"context"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/libp2p/go-msgio"
	"go.opencensus.io/stats"
	"go.opencensus.io/tag"
	"golang.org/x/xerrors"

	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/submodule/metrics"
)

var dhtReadMessageTimeout = 15 * time.Second

var logger = logging.Logger("net-message")

type MessageSender interface {
	// SendRequest sends a peer a message and waits for its response
	SendRequest(ctx context.Context, p peer.ID, pmes *pb.NetMessage) (*pb.NetMessage, error)
	// SendMessage sends a peer a message without waiting on a response
	SendMessage(ctx context.Context, p peer.ID, pmes *pb.NetMessage) error
}

// messageSenderImpl is responsible for sending requests and messages to peers efficiently, including reuse of streams.
// It also tracks metrics for sent requests and messages.
type messageSenderImpl struct {
	host      host.Host // the network services we need
	smlk      sync.Mutex
	strmap    map[peer.ID]*peerMessageSender
	protocols []protocol.ID
}

func NewMessageSenderImpl(h host.Host, protos []protocol.ID) MessageSender {
	return &messageSenderImpl{
		host:      h,
		strmap:    make(map[peer.ID]*peerMessageSender),
		protocols: protos,
	}
}

func (m *messageSenderImpl) OnDisconnect(ctx context.Context, p peer.ID) {
	m.smlk.Lock()
	defer m.smlk.Unlock()
	ms, ok := m.strmap[p]
	if !ok {
		return
	}
	delete(m.strmap, p)

	// Do this asynchronously as ms.lk can block for a while.
	go func() {
		err := ms.lk.Lock(ctx)
		if err != nil {
			return
		}
		defer ms.lk.Unlock()
		ms.invalidate()
	}()
}

func (m *messageSenderImpl) SendRequest(ctx context.Context, p peer.ID, pmes *pb.NetMessage) (*pb.NetMessage, error) {
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.NetMessageType, pmes.GetHeader().GetType().String()))

	ms, err := m.messageSenderForPeer(ctx, p)
	if err != nil {
		stats.Record(ctx,
			metrics.NetSentRequests.M(1),
			metrics.NetSentRequestErrors.M(1),
		)
		logger.Debugw("request failed to open message sender", "error", err, "to", p)
		return nil, err
	}

	start := time.Now()

	rpmes, err := ms.SendRequest(ctx, pmes)
	if err != nil {
		stats.Record(ctx,
			metrics.NetSentRequests.M(1),
			metrics.NetSentRequestErrors.M(1),
		)
		logger.Debugw("request failed", "error", err, "to", p)
		return nil, err
	}

	stats.Record(ctx,
		metrics.NetSentRequests.M(1),
		metrics.NetSentBytes.M(int64(pmes.Size())),
		metrics.NetOutboundRequestLatency.M(float64(time.Since(start))/float64(time.Millisecond)),
	)

	m.host.Peerstore().RecordLatency(p, time.Since(start))
	return rpmes, nil
}

// SendMessage sends out a message
func (m *messageSenderImpl) SendMessage(ctx context.Context, p peer.ID, pmes *pb.NetMessage) error {
	ms, err := m.messageSenderForPeer(ctx, p)
	if err != nil {
		logger.Debugw("message failed to open message sender", "error", err, "to", p)
		return err
	}

	err = ms.SendMessage(ctx, pmes)
	if err != nil {
		logger.Debugw("message failed", "error", err, "to", p)
		return err
	}

	return nil
}

func (m *messageSenderImpl) messageSenderForPeer(ctx context.Context, p peer.ID) (*peerMessageSender, error) {
	m.smlk.Lock()
	ms, ok := m.strmap[p]
	if ok {
		m.smlk.Unlock()
		return ms, nil
	}
	ms = &peerMessageSender{p: p, m: m, lk: NewCtxMutex()}
	m.strmap[p] = ms
	m.smlk.Unlock()

	err := ms.prepOrInvalidate(ctx)
	if err != nil {
		m.smlk.Lock()
		defer m.smlk.Unlock()

		if msCur, ok := m.strmap[p]; ok {
			// Changed. Use the new one, old one is invalid and
			// not in the map so we can just throw it away.
			if ms != msCur {
				return msCur, nil
			}
			// Not changed, remove the now invalid stream from the
			// map.
			delete(m.strmap, p)
		}
		// Invalid but not in map. Must have been removed by a disconnect.
		return nil, err
	}
	// All ready to go.
	return ms, nil
}

// peerMessageSender is responsible for sending requests and messages to a particular peer
type peerMessageSender struct {
	s  network.Stream
	r  msgio.ReadCloser
	lk CtxMutex
	p  peer.ID
	m  *messageSenderImpl

	invalid   bool
	singleMes int
}

// invalidate is called before this peerMessageSender is removed from the strmap.
// It prevents the peerMessageSender from being reused/reinitialized and then
// forgotten (leaving the stream open).
func (ms *peerMessageSender) invalidate() {
	ms.invalid = true
	if ms.s != nil {
		_ = ms.s.Reset()
		ms.s = nil
	}
}

func (ms *peerMessageSender) prepOrInvalidate(ctx context.Context) error {
	err := ms.lk.Lock(ctx)
	if err != nil {
		return err
	}
	defer ms.lk.Unlock()

	err = ms.prep(ctx)
	if err != nil {
		ms.invalidate()
		return err
	}
	return nil
}

func (ms *peerMessageSender) prep(ctx context.Context) error {
	if ms.invalid {
		return xerrors.Errorf("message sender has been invalidated")
	}
	if ms.s != nil {
		return nil
	}

	// We only want to speak to peers using our primary protocols. We do not want to query any peer that only speaks
	// one of the secondary "server" protocols that we happen to support (e.g. older nodes that we can respond to for
	// backwards compatibility reasons).
	nstr, err := ms.m.host.NewStream(ctx, ms.p, ms.m.protocols...)
	if err != nil {
		return err
	}

	ms.r = msgio.NewVarintReaderSize(nstr, network.MessageSizeMax)
	ms.s = nstr

	return nil
}

// streamReuseTries is the number of times we will try to reuse a stream to a
// given peer before giving up and reverting to the old one-message-per-stream
// behaviour.
const streamReuseTries = 3

func (ms *peerMessageSender) SendMessage(ctx context.Context, pmes *pb.NetMessage) error {
	err := ms.lk.Lock(ctx)
	if err != nil {
		return err
	}
	defer ms.lk.Unlock()

	retry := false
	for {
		err = ms.prep(ctx)
		if err != nil {
			return err
		}

		err = ms.writeMsg(pmes)
		if err != nil {
			_ = ms.s.Reset()
			ms.s = nil

			if retry {
				logger.Debugw("error writing message", "error", err)
				return err
			}
			logger.Debugw("error writing message", "error", err, "retrying", true)
			retry = true
			continue
		}

		var err error
		if ms.singleMes > streamReuseTries {
			err = ms.s.Close()
			ms.s = nil
		} else if retry {
			ms.singleMes++
		}

		return err
	}
}

func (ms *peerMessageSender) SendRequest(ctx context.Context, pmes *pb.NetMessage) (*pb.NetMessage, error) {
	err := ms.lk.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer ms.lk.Unlock()

	retry := false
	for {
		err = ms.prep(ctx)
		if err != nil {
			return nil, err
		}

		err = ms.writeMsg(pmes)
		if err != nil {
			_ = ms.s.Reset()
			ms.s = nil

			if retry {
				logger.Debugw("error writing message", "error", err)
				return nil, err
			}
			logger.Debugw("error writing message", "error", err, "retrying", true)
			retry = true
			continue
		}

		mes := new(pb.NetMessage)
		err = ms.ctxReadMsg(ctx, mes)
		if err != nil {
			_ = ms.s.Reset()
			ms.s = nil

			if retry {
				logger.Debugw("error reading message", "error", err)
				return nil, err
			}
			logger.Debugw("error reading message", "error", err, "retrying", true)
			retry = true
			continue
		}

		var err error
		if ms.singleMes > streamReuseTries {
			err = ms.s.Close()
			ms.s = nil
		} else if retry {
			ms.singleMes++
		}

		return mes, err
	}
}

func (ms *peerMessageSender) writeMsg(pmes *pb.NetMessage) error {
	return WriteMsg(ms.s, pmes)
}

func (ms *peerMessageSender) ctxReadMsg(ctx context.Context, mes *pb.NetMessage) error {
	errc := make(chan error, 1)
	go func(r msgio.ReadCloser) {
		defer close(errc)
		bytes, err := r.ReadMsg()
		defer r.ReleaseMsg(bytes)
		if err != nil {
			errc <- err
			return
		}
		errc <- mes.Unmarshal(bytes)
	}(ms.r)

	t := time.NewTimer(dhtReadMessageTimeout)
	defer t.Stop()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return xerrors.New("timed out reading response")
	}
}
