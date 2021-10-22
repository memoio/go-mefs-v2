package node

import (
	libp2p "github.com/libp2p/go-libp2p"
	ci "github.com/libp2p/go-libp2p-core/crypto"
	errors "github.com/pkg/errors"

	minit "github.com/memoio/go-mefs-v2/lib/init"
	"github.com/memoio/go-mefs-v2/lib/repo"
)

// OptionsFromRepo takes a repo and returns options that configure a node
// to use the given repo.
func OptionsFromRepo(r repo.Repo) ([]BuilderOpt, error) {
	sk, err := privKeyFromKeystore(r)
	if err != nil {
		return nil, err
	}

	cfg := r.Config()
	cfgopts := []BuilderOpt{
		// Libp2pOptions can only be called once, so add all options here.
		Libp2pOptions(
			libp2p.ListenAddrStrings(cfg.Net.Addresses...),
			libp2p.Identity(sk),
		),
	}

	dsopt := func(c *Builder) error {
		c.repo = r
		return nil
	}

	return append(cfgopts, dsopt), nil
}

func privKeyFromKeystore(r repo.Repo) (ci.PrivKey, error) {
	ki, err := r.KeyStore().Get(minit.SelfNetKey, "")
	if err != nil {
		return nil, errors.Wrap(err, "failed to get key from keystore")
	}

	sk, err := ci.UnmarshalPrivateKey(ki.SecretKey)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal private key failed")
	}
	return sk, nil
}
