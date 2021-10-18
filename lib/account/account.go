package account

import (
	"sync"

	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/types"
)

type Account struct {
	types.KeyInfo
	Address address.Address
}

type LocalWallet struct {
	sync.Mutex
	password string                       // used for decrypt
	accounts map[address.Address]*Account // from address to its account
	keystore types.KeyStore               // store
}

type Wallet interface {
	WalletSign()
	WalletVerify()
	WalletList()
	WalletExport()
	WalletChainAddress()
}
