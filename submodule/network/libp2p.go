package network

import (
	"crypto/rand"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/pkg/errors"

	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/types"
)

var logger = logging.Logger("netModule")

const (
	SelfNetKey = "libp2p-self"
)

// create or load net private key
func GetSelfNetKey(store types.KeyStore) (peer.ID, crypto.PrivKey, error) {
	ki, err := store.Get(SelfNetKey, SelfNetKey)
	if err == nil {
		sk, err := crypto.UnmarshalPrivateKey(ki.SecretKey)
		if err != nil {
			return peer.ID(""), nil, err
		}

		p, err := peer.IDFromPublicKey(sk.GetPublic())
		if err != nil {
			return peer.ID(""), nil, errors.Wrap(err, "failed to get peer ID")
		}

		logger.Info("load local peerID: ", p.Pretty())

		return p, sk, nil
	}

	// ed25519 30% faster than secp256k1
	logger.Info("generating ED25519 keypair for p2p network...")
	sk, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return peer.ID(""), nil, errors.Wrap(err, "failed to create peer key")
	}

	data, err := sk.Bytes()
	if err != nil {
		return peer.ID(""), nil, err
	}

	nki := types.KeyInfo{
		SecretKey: data,
		Type:      types.Ed25519,
	}

	if err := store.Put(SelfNetKey, SelfNetKey, nki); err != nil {
		return peer.ID(""), nil, errors.Wrap(err, "failed to store private key")
	}

	p, err := peer.IDFromPublicKey(sk.GetPublic())
	if err != nil {
		return peer.ID(""), nil, errors.Wrap(err, "failed to get peer ID")
	}

	logger.Info("generated peerID: ", p.Pretty())
	return p, sk, nil
}
