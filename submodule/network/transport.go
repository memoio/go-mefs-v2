package network

import (
	"github.com/libp2p/go-libp2p"
	noise "github.com/libp2p/go-libp2p-noise"
	tls "github.com/libp2p/go-libp2p-tls"
)

func Transport() libp2p.Option {
	return libp2p.ChainOptions(libp2p.DefaultTransports)
}

func Security(enabled, preferTLS bool) libp2p.Option {
	if !enabled {
		return libp2p.NoSecurity
	}
	if preferTLS {
		return libp2p.ChainOptions(libp2p.Security(tls.ID, tls.New), libp2p.Security(noise.ID, noise.New))
	} else {
		return libp2p.ChainOptions(libp2p.Security(noise.ID, noise.New), libp2p.Security(tls.ID, tls.New))
	}
}
