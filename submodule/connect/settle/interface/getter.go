package inter

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/memoio/go-mefs-v2/api"
)

type RoleOut struct {
	State        uint8
	RType        uint8
	Index        uint64
	GIndex       uint64
	RegisterTime *big.Int
	Owner        common.Address
	Next         common.Address
	VerifyKey    []byte
	Desc         []byte
}

type StoreOut struct {
	Start  uint64
	End    uint64
	Size   uint64
	Sprice *big.Int
}

type SettleOut struct {
	Time    uint64
	Size    uint64
	Price   *big.Int
	MaxPay  *big.Int
	HasPaid *big.Int
	CanPay  *big.Int
	Lost    *big.Int
}

type FsOut struct {
	Nonce    uint64
	SubNonce uint64
}

type GroupOut struct {
	Size   uint64
	Sprice *big.Int
	Lost   *big.Int
}

type RewardOut struct {
	Accu       *big.Int
	Last       *big.Int
	Pledge     *big.Int
	Reward     *big.Int
	CurReward  *big.Int
	PledgeTime *big.Int
}

type IGetter interface {
	// token related
	GetToken(ti uint8) (common.Address, error)

	// role Related
	GetAddrCnt() uint64
	GetAddrAt(i uint64) (common.Address, error)
	GetRoleInfo(addr common.Address) (*RoleOut, error)

	GetGroupInfo(gi uint64) (*api.GroupInfo, error)

	// pledge related
	GetPledgePool() (common.Address, error)
	GetTotalPledge() (*big.Int, error)
	GetPledge(ti uint8) (*big.Int, error)
	GetPledgeAt(i uint64, ti uint8) (*big.Int, error)

	// fs related
	GetFsPool() (common.Address, error)
	GetBalAt(i uint64, ti uint8) (*big.Int, *big.Int, *big.Int, error)
	GetStoreInfo(ui, pi uint64, ti uint8) (*StoreOut, error)
	GetSettleInfo(pi uint64, ti uint8) (*SettleOut, error)
	GetFsInfo(ui, pi uint64) (*FsOut, error)
	GetGInfo(gi uint64, ti uint8) (*GroupOut, error)

	// add some info
	GetPleRewardInfo(index uint64, ti uint8) (*RewardOut, error)
}
