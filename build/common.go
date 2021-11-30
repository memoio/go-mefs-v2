package build

import "math/big"

const (
	DefaultSegSize      = 248 * 1024
	DefaultChalDuration = 20
)

var (
	DefaultSegPrice   = big.NewInt(1000) // per seg
	DefaultPiecePrice = big.NewInt(1000)
)
