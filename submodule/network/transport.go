package network

import (
	"github.com/libp2p/go-libp2p"
	metrics "github.com/libp2p/go-libp2p-core/metrics"
	libp2pquic "github.com/libp2p/go-libp2p-quic-transport"
	tcp "github.com/libp2p/go-tcp-transport"
	websocket "github.com/libp2p/go-ws-transport"
	config "github.com/memoio/go-mefs-v2/config"
)

func Transports(tptConfig config.Transports, opts []libp2p.Option) []libp2p.Option {
	if tptConfig.Network.TCP.WithDefault(true) {
		opts = append(opts, libp2p.Transport(tcp.NewTCPTransport))
	}

	if tptConfig.Network.Websocket.WithDefault(true) {
		opts = append(opts, libp2p.Transport(websocket.New))
	}

	if tptConfig.Network.QUIC.WithDefault(true) {
		opts = append(opts, libp2p.Transport(libp2pquic.NewTransport))
	}

	return opts
}

func BandwidthCounter(opts []libp2p.Option) ([]libp2p.Option, *metrics.BandwidthCounter) {
	reporter := metrics.NewBandwidthCounter()
	opts = append(opts, libp2p.BandwidthReporter(reporter))
	return opts, reporter
}
