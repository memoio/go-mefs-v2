package inter

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

type OrderIn struct {
	UIndex uint64
	PIndex uint64
	Start  uint64
	End    uint64
	Size   uint64
	Nonce  uint64
	TIndex uint8
	Sprice *big.Int
}

type PWIn struct {
	PIndex uint64
	TIndex uint8
	Pay    *big.Int
	Lost   *big.Int
}

type IProxy interface {
	// register self to get index
	ReAcc() error
	ReRole(rtype uint8, extra []byte) error
	QuitRole(rid uint64) error

	AlterPayee(rid uint64, p common.Address) error

	// add a user/keeper/provider to group
	AddToGroup(gi uint64) error
	SetDesc(desc []byte) error

	Pledge(i uint64, money *big.Int) error
	PledgeWithdraw(i uint64, ti uint8, money *big.Int) error
	PledgeRewardWithdraw(i uint64, ti uint8, money *big.Int) error

	AddOrder(oi OrderIn, uSign []byte, pSign []byte) error
	SubOrder(oi OrderIn, uSign []byte, pSign []byte) error

	AddRepair(oi OrderIn, kis []uint64, ksigns [][]byte) error

	Recharge(i uint64, ti uint8, isLock bool, money *big.Int) error
	Withdraw(i uint64, ti uint8, money *big.Int) error
	ProWithdraw(ps PWIn, kis []uint64, ksigns [][]byte) error
}
