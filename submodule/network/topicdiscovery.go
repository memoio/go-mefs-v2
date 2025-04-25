package network

import (
	"context"
	"math/rand"
	"time"

	"github.com/libp2p/go-libp2p/core/discovery"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/routing"
	discbackoff "github.com/libp2p/go-libp2p/p2p/discovery/backoff"
	discrouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
)

func TopicDiscovery(ctx context.Context, host host.Host, cr routing.Routing) (service discovery.Discovery, err error) {
	baseDisc := discrouting.NewRoutingDiscovery(cr)
	minBackoff, maxBackoff := time.Second*60, time.Hour
	rng := rand.New(rand.NewSource(rand.Int63()))
	d, err := discbackoff.NewBackoffDiscovery(
		baseDisc,
		discbackoff.NewExponentialBackoff(minBackoff, maxBackoff, discbackoff.FullJitter, time.Second, 5.0, 0, rng),
	)

	if err != nil {
		return nil, err
	}

	return d, nil
}
