package build

import (
	"math/big"
	"time"
)

const (
	DefaultSegSize = 248 * 1024   // byte
	SlotDuration   = 30           // seconds
	OrderMin       = 1 * 86400    // min 100days
	OrderMax       = 1000 * 86400 // max 1000 days
)

// version 0
const (
	ChalDuration0 = 120 // slot
)

// version 1
const (
	UpdateEpoch1  = 248780
	ChalDuration1 = 360 // slot
)

const (
	UpdateEpoch2  = 259600
	ChalDuration2 = 960 // slot
)

var (
	DefaultSegPrice   = big.NewInt(1000) // per seg
	DefaultPiecePrice = big.NewInt(1000)
	BaseTime          = time.Date(2021, time.December, 1, 0, 0, 0, 0, time.UTC).Unix()
)
