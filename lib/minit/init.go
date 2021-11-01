package minit

import (
	"context"
	"encoding/binary"
	"fmt"
	"strconv"

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

	addr, err := w.WalletNew(ctx, types.Secp256k1)
	if err != nil {
		return err
	}

	fmt.Println("addr: ", addr.String())

	id := binary.BigEndian.Uint64(addr.Bytes()[:8])

	cfg := r.Config()
	cfg.Identity.Name = strconv.Itoa(int(id))
	cfg.Wallet.DefaultAddress = addr.String()

	return nil
}
