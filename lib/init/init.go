package init

import (
	"context"
	"fmt"

	acrypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/pkg/errors"

	"github.com/memoio/go-mefs-v2/lib/account"
	"github.com/memoio/go-mefs-v2/lib/repo"
	"github.com/memoio/go-mefs-v2/lib/types"
)

const defaultPeerKeyBits = 2048

const SelfNetKey = "selfnetkey"

func Init(ctx context.Context, r repo.Repo, password string) error {

	if err := CreatePeerNetKey(r.KeyStore(), password); err != nil {
		return err
	}

	w := account.NewWallet(password, r.KeyStore())

	fmt.Println("generating secp256k1 keypair...")

	addr, err := w.WalletNew(types.Secp256k1)
	if err != nil {
		return err
	}

	fmt.Println("addr: ", addr.String())

	config := r.Config()
	config.Wallet.DefaultAddress = addr.String()

	return nil
}

func CreatePeerNetKey(store types.KeyStore, password string) error {
	fmt.Println("generating ED25519 keypair for p2p network...")
	key, _, err := acrypto.GenerateKeyPair(acrypto.Ed25519, defaultPeerKeyBits)
	if err != nil {
		return errors.Wrap(err, "failed to create peer key")
	}

	data, err := key.Raw()
	if err != nil {
		return err
	}

	ki := types.KeyInfo{
		SecretKey: data,
		Type:      types.Ed25519,
	}

	if err := store.Put(SelfNetKey, password, ki); err != nil {
		return errors.Wrap(err, "failed to store private key")
	}

	p, err := peer.IDFromPublicKey(key.GetPublic())
	if err != nil {
		return errors.Wrap(err, "failed to get peer ID")
	}

	fmt.Println("peer identity: ", p.Pretty())
	return nil
}
