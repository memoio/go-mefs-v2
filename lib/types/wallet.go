package types

import (
	"github.com/memoio/go-mefs-v2/lib/address"
	sig_common "github.com/memoio/go-mefs-v2/lib/crypto/signature/common"
)

type Wallet interface {
	WalletNew(sig_common.KeyType) (address.Address, error)
	WalletSign(addr address.Address, msg []byte) ([]byte, error)
	WalletList() ([]address.Address, error)
	WalletHas(address.Address) bool
	WalletDelete(address.Address) error
	WalletExport(addr address.Address) (*KeyInfo, error)
	WalletImport(ki *KeyInfo) (address.Address, error)
}
