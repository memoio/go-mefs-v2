package wallet

import (
	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/types"
)

type Wallet interface {
	WalletNew(types.KeyType) (address.Address, error)
	WalletSign(addr address.Address, msg []byte) ([]byte, error)
	WalletList() ([]address.Address, error)
	WalletHas(address.Address) bool
	WalletDelete(address.Address) error
	WalletExport(addr address.Address) (*types.KeyInfo, error)
	WalletImport(ki *types.KeyInfo) (address.Address, error)
}
