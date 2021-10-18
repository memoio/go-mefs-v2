package types

import "math/big"

// for order verify: nonce. seqnum, size, price
type OrderManage struct {
	nonce      uint64 // current nonce
	segPrice   *big.Int
	piecePrice *big.Int
	start      uint64
	end        uint64
	seqNum     uint32 // currnet seqnum
	pSize      uint64
	seqSize    uint64
	size       uint64
	price      *big.Int
}

// cost per fs
type FSPay struct {
	order      OrderManage // current order
	pSize      uint64      // total piece size; add when OrderManage.pSize change
	piecePrice *big.Int    // average price on pvsize

	segSize  uint64   // total segment size; add when OrderManage.segSize change
	segPrice *big.Int // average price on segSize

	expectedCost *big.Int // expected cost of lifecycle; accumulated
}

type PayForProvider struct {
	expectedCost *big.Int // expected income of lifecycle; accumulated

	size  uint64   // total piece+segment size
	price *big.Int // average price

	pSize uint64 // 聚合

	pvSize     uint64   // verifed; when commit data and sector
	piecePrice *big.Int // average price on pvsize

	pay map[uint64]*FSPay // key: fsID
}

type PayMange struct {
	proEarn map[uint64]*PayForProvider // pay for provider
}

// accumated pay info at each chal epoch
type ChalPay struct {
	res       bool   // segment challenge ok
	accHw     []byte // accumulated fr
	lostAccHw []byte // accumulated fr for lost segment
	proof     []byte
	segPay    *big.Int // sub lost of this epoch
	lost      *big.Int
}

type ChalPayForProvider struct {
	res        bool // sector challeng ok
	proof      []byte
	piecePay   *big.Int
	pieceLost  *big.Int
	sectorMax  uint64
	sectorLost []uint64

	pay map[uint64]*ChalPay // per fs

	total *big.Int
	lost  *big.Int // money due to unable to response to chal
}

// 从结算层拿钱
type PayOnSettle struct {
	proID uint64
	total *big.Int // accumulated value
	lost  *big.Int
	sign  [][]byte // keeper签名
}

type ChalPayManage struct {
	seed ChalSeed
	chal map[uint64]*ChalPayForProvider // per provider
	paid map[uint64]*PayOnSettle        // per provider
}

type ChalSeed struct {
	epoch uint64
	time  uint64
	seed  []byte // vrf bytes or block.root?
}
