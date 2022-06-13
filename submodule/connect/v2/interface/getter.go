package inter

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/memoio/contractsv2/go_contracts/getter"
	"github.com/memoio/go-mefs-v2/api"
)

type IGetter interface {
	// token related
	GetToken(ti uint8) (common.Address, error)

	// role Related
	GetAddrCnt() uint64
	GetAddrAt(i uint64) (common.Address, error)
	GetRoleInfo(addr common.Address) (*getter.RoleOut, error)

	GetGroupInfo(gi uint64) (*api.GroupInfo, error)

	// pledge related
	GetPledgePool() (common.Address, error)
	GetTotalPledge() *big.Int
	GetPledge(ti uint8) *big.Int
	GetPledgeAt(i uint64, ti uint8) *big.Int

	// fs related
	GetFsPool() (common.Address, error)
	GetBalAt(i uint64, ti uint8) (*big.Int, *big.Int)
	GetStoreInfo(ui, pi uint64, ti uint8) *getter.StoreOut
	GetSettleInfo(pi uint64, ti uint8) *getter.SettleOut
	GetFsInfo(ui, pi uint64) *getter.FsOut
	GetGInfo(gi uint64, ti uint8) *getter.GroupOut
}
