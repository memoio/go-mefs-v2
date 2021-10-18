package network

import (
	"os"

	"github.com/libp2p/go-libp2p"
	smux "github.com/libp2p/go-libp2p-core/mux"
	mplex "github.com/libp2p/go-libp2p-mplex"
	yamux "github.com/libp2p/go-libp2p-yamux"

	config "github.com/memoio/go-mefs-v2/config"
)

func yamuxTransport() smux.Multiplexer {
	tpt := *yamux.DefaultTransport
	tpt.AcceptBacklog = 512
	if os.Getenv("YAMUX_DEBUG") != "" {
		tpt.LogOutput = os.Stderr
	}

	return &tpt
}

func makeSmuxTransportOption(tptConfig config.Transports) libp2p.Option {
	const yamuxID = "/yamux/1.0.0"
	const mplexID = "/mplex/6.7.0"

	ymxtpt := *yamux.DefaultTransport
	ymxtpt.AcceptBacklog = 512

	return prioritizeOptions([]priorityOption{{
		priority:        tptConfig.Multiplexers.Yamux,
		defaultPriority: 100,
		opt:             libp2p.Muxer(yamuxID, yamuxTransport),
	}, {
		priority:        tptConfig.Multiplexers.Mplex,
		defaultPriority: 200,
		opt:             libp2p.Muxer(mplexID, mplex.DefaultTransport),
	}})
}
