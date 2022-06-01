package inter

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/memoio/contractsv2/go_contracts/fs"
	"github.com/memoio/contractsv2/go_contracts/role"
)

type GroupInfo struct {
	IsActive bool
	IsBan    bool
	Level    uint8
	KManage  common.Address
	Kpr      *big.Int
	Ppr      *big.Int
}

type IGetter interface {
	// role Related
	GetAddrCnt() uint64
	GetAddrAt(i uint64) (common.Address, error)
	GetRoleInfo(addr common.Address) (*role.RoleOut, error)

	GetGroupInfo(gi uint64) (*GroupInfo, error)

	// pledge related
	GetPledgePool() (common.Address, error)
	GetTotalPledge() *big.Int
	GetPledge(ti uint8) *big.Int
	GetPledgeAt(i uint64, ti uint8) *big.Int

	// fs related
	GetFsPool() (common.Address, error)
	GetBalAt(i uint64, ti uint8) *big.Int
	GetStoreInfo(ui, pi uint64, ti uint8) *fs.StoreOut
	GetSettleInfo(pi uint64, ti uint8) *fs.SettleOut
	GetFsInfo(ui, pi uint64) *fs.FsOut
	GetGInfo(gi uint64, ti uint8) *fs.GroupOut
}
