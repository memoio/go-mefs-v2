package order

import "math/big"

//
type ParasBase struct {
	Addr []byte
	Sig  []byte
}

type ParasCommon struct {
	Index      uint64
	GIndex     uint64
	TokenIndex uint32
	Amount     *big.Int
	Extra      []byte
}

type SignedParasCommon struct {
	ParasCommon
	Auth [][]byte // signed by index; meta-transcation; or auth by keepers/admin
}

type ParasOrder struct {
	User       uint64
	Provider   uint64
	Start      uint64
	End        uint64
	Size       uint64
	Nonce      uint64
	TokenIndex uint32
	Price      *big.Int
}

type SignedParasOrder struct {
	ParasOrder
	Usign []byte
	PSign []byte
	Auth  [][]byte
}

// key: 'Order'/user/pro; value: nonce
// key: 'Order'/user/pro/nonce; value: content
type Order struct {
	UserID     uint64
	ProID      uint64
	Nonce      uint64
	Start      uint64
	End        uint64
	Size       uint64
	TokenIndex uint32
	Price      *big.Int
	UserSig    []byte
	ProSig     []byte
}

// sent to settle

// AddOrder and SubOrder
type SignedOrder struct {
	Order
	Auth [][]byte
}
type ParasProWithdraw struct {
	ProID      uint64
	TokenIndex uint32
	Amount     *big.Int
	Lost       *big.Int
}

type SignedParasProWithdraw struct {
	ParasProWithdraw
	Auth [][]byte
}
