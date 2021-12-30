package build

import (
	"math/big"
	"time"
)

const (
	DefaultSegSize      = 248 * 1024    // byte
	DefaultChalDuration = 30            // slot
	SlotDuration        = 30            // seconds
	OrderDuration       = 10 * 86400    // 1 days
	OrderMin            = OrderDuration // min 100days
	OrderMax            = OrderDuration // max 1000 days
)

var (
	DefaultSegPrice   = big.NewInt(1000) // per seg
	DefaultPiecePrice = big.NewInt(1000)
	BaseTime          = time.Date(2021, time.December, 1, 0, 0, 0, 0, time.UTC).Unix()
)
