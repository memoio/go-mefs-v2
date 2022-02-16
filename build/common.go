package build

import (
	"math/big"
	"time"
)

const (
	DefaultSegSize      = 248 * 1024    // byte
	DefaultChalDuration = 120           // slot
	SlotDuration        = 30            // seconds
	OrderDuration       = 10 * OrderMin // 1 day for test
	OrderMin            = 1 * 86400     // min 100days
	OrderMax            = 1000 * 86400  // max 1000 days
	Version             = 1
)

var (
	DefaultSegPrice   = big.NewInt(1000) // per seg
	DefaultPiecePrice = big.NewInt(1000)
	BaseTime          = time.Date(2021, time.December, 1, 0, 0, 0, 0, time.UTC).Unix()
)
