package build

import (
	"math/big"
	"time"
)

const (
	DefaultSegSize      = 248 * 1024
	DefaultChalDuration = 20
)

var (
	DefaultSegPrice   = big.NewInt(1000) // per seg
	DefaultPiecePrice = big.NewInt(1000)
	BaseTime          = time.Date(2021, time.December, 1, 0, 0, 0, 0, time.UTC).Unix()
)
