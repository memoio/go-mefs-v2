package network

import (
	"context"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/host"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

func FloodSub(ctx context.Context, host host.Host, disc discovery.Discovery, pubsubOptions ...pubsub.Option) (service *pubsub.PubSub, err error) {
	return pubsub.NewFloodSub(ctx, host, append(pubsubOptions, pubsub.WithDiscovery(disc))...)

}

func GossipSub(ctx context.Context, host host.Host, disc discovery.Discovery, pubsubOptions ...pubsub.Option) (service *pubsub.PubSub, err error) {
	return pubsub.NewGossipSub(ctx, host, append(
		pubsubOptions,
		pubsub.WithDiscovery(disc),
		pubsub.WithFloodPublish(true))...,
	)
}
