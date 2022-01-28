package network

import (
	"context"
	"math/rand"
	"time"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/routing"
	disc "github.com/libp2p/go-libp2p-discovery"
)

func TopicDiscovery(ctx context.Context, host host.Host, cr routing.Routing) (service discovery.Discovery, err error) {
	baseDisc := disc.NewRoutingDiscovery(cr)
	minBackoff, maxBackoff := time.Second*60, time.Hour
	rng := rand.New(rand.NewSource(rand.Int63()))
	d, err := disc.NewBackoffDiscovery(
		baseDisc,
		disc.NewExponentialBackoff(minBackoff, maxBackoff, disc.FullJitter, time.Second, 5.0, 0, rng),
	)

	if err != nil {
		return nil, err
	}

	return d, nil
}
