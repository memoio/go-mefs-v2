package network

import (
	"crypto/rand"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/xerrors"

	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/types"
)

var logger = logging.Logger("net-p2p")

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
			return peer.ID(""), nil, xerrors.Errorf("failed to get peer ID %w", err)
		}

		logger.Info("load local peer: ", p.Pretty())

		return p, sk, nil
	}

	// ed25519 30% faster than secp256k1
	logger.Info("generating ED25519 keypair for p2p network...")
	sk, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return peer.ID(""), nil, xerrors.Errorf("failed to create peer key %w", err)
	}

	data, err := sk.Bytes()
	if err != nil {
		return peer.ID(""), nil, err
	}

	nki := types.KeyInfo{
		SecretKey: data,
		Type:      types.Ed25519,
	}

	err = store.Put(SelfNetKey, SelfNetKey, nki)
	if err != nil {
		return peer.ID(""), nil, xerrors.Errorf("failed to store private key %w", err)
	}

	p, err := peer.IDFromPublicKey(sk.GetPublic())
	if err != nil {
		return peer.ID(""), nil, xerrors.Errorf("failed to get peer ID %w", err)
	}

	logger.Info("generated p2p network peer: ", p.Pretty())
	return p, sk, nil
}
