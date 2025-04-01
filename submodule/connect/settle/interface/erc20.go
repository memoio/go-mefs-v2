package inter

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

type IERC20 interface {
	Transfer(recipient common.Address, amount *big.Int) error
	Approve(spender common.Address, amount *big.Int) error

	BalanceOf(account common.Address) *big.Int
	Allowance(owner common.Address, spender common.Address) *big.Int
}
