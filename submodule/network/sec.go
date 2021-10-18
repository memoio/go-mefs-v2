package network

import (
	"github.com/libp2p/go-libp2p"
	noise "github.com/libp2p/go-libp2p-noise"
	secio "github.com/libp2p/go-libp2p-secio"
	tls "github.com/libp2p/go-libp2p-tls"

	config "github.com/memoio/go-mefs-v2/config"
)

func Security(tptConfig config.Transports, opts []libp2p.Option) []libp2p.Option {
	// Using the new config options.
	opts = append(opts, prioritizeOptions([]priorityOption{{
		priority:        tptConfig.Security.TLS,
		defaultPriority: 100,
		opt:             libp2p.Security(tls.ID, tls.New),
	}, {
		priority:        tptConfig.Security.SECIO,
		defaultPriority: config.Disabled,
		opt:             libp2p.Security(secio.ID, secio.New),
	}, {
		priority:        tptConfig.Security.Noise,
		defaultPriority: 300,
		opt:             libp2p.Security(noise.ID, noise.New),
	}}))
	return opts

}
