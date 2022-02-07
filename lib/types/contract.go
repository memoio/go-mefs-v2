package types

import (
	"math/big"
)

type ChannelPay struct {
	Addr     []byte
	UserID   uint64
	ProID    uint64
	Nonce    uint64
	Start    int64
	Duration int64
	Money    *big.Int
	Value    *big.Int
}

// for settle sign
func (cp *ChannelPay) Hash() []byte {
	return nil
}

func (cp *ChannelPay) Serialize() ([]byte, error) {
	return nil, nil
}

func (cp *ChannelPay) Deserialize(b []byte) error {
	return nil
}

type SignedChannelPay struct {
	ChannelPay
	Sig Signature
}

func (scp *SignedChannelPay) Hash() MsgID {
	return MsgIDUndef
}
