package build

import (
	"math/big"
	"time"
)

const (
	DefaultSegSize = 248 * 1024   // byte
	SlotDuration   = 30           // seconds
	OrderMin       = 100 * 86400  // min 100 days
	OrderMax       = 1000 * 86400 // max 1000 days
)

// version 0
const (
	ChalDuration0 = 120 // slot, 1h
	OrderMin0     = 1 * 86400
)

// version 1
const (
	UpdateHeight1 = 2880
	ChalDuration1 = 360 // slot, 3h
	OrderMin1     = 1 * 86400
)

const (
	UpdateHeight2 = 2880 * 5
	ChalDuration2 = 960 // slot, 8h
	OrderMin2     = 1 * 86400
)

const (
	UpdateHeight3 = 2880 * 14
	ChalDuration3 = 2880 // slot, 24h
	OrderMin3     = OrderMin
)

var (
	DefaultSegPrice   = big.NewInt(1000) // per seg
	DefaultPiecePrice = big.NewInt(1000)
	BaseTime          = time.Date(2021, time.December, 1, 0, 0, 0, 0, time.UTC).Unix()
)
