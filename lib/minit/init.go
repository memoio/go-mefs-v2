package minit

import (
	"context"
	"fmt"

	"github.com/memoio/go-mefs-v2/lib/repo"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/submodule/network"
	"github.com/memoio/go-mefs-v2/submodule/wallet"
)

// init ops for mefs
func Init(ctx context.Context, r repo.Repo, password string) error {

	if _, _, err := network.GetSelfNetKey(r.KeyStore()); err != nil {
		return err
	}

	w := wallet.New(password, r.KeyStore())

	fmt.Println("generating wallet address...")

	addr, err := w.WalletNew(types.Secp256k1)
	if err != nil {
		return err
	}

	fmt.Println("addr: ", addr.String())

	config := r.Config()
	config.Wallet.DefaultAddress = addr.String()

	return nil
}
