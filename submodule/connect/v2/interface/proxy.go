package inter

import (
	"math/big"

	"github.com/memoio/contractsv2/go_contracts/proxy"
)

type IProxy interface {
	// register self to get index
	ReAcc() error
	ReRole(rtype uint8, extra []byte) error

	// add a user/keeper/provider to group
	AddToGroup(gi uint64) error
	SetDesc(desc []byte) error

	Pledge(i uint64, money *big.Int) error
	PledgeWithdraw(i uint64, ti uint8, money *big.Int) error

	AddOrder(oi proxy.OrderIn, uSign []byte, pSign []byte) error
	SubOrder(oi proxy.OrderIn, uSign []byte, pSign []byte) error

	AddRepair(oi proxy.OrderIn, kis []uint64, ksigns [][]byte) error

	Recharge(i uint64, ti uint8, isLock bool, money *big.Int) error
	Withdraw(i uint64, ti uint8, money *big.Int) error
	ProWithdraw(ps proxy.PWIn, kis []uint64, ksigns [][]byte) error
}
