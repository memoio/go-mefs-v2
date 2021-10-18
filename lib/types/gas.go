package types

import (
	"math/big"
)

// GasMoney get from settlement chain; add/sub depends on data chain
type GasMoney struct {
	OnChain  *big.Int // >0
	Profit   *big.Int // >0 earn from message fee; or <0 cost for message fee; =/- (gasPrice*gasLimit+value)
	Withdraw *big.Int // has withdrawed; >0
}

func (g GasMoney) Serialize() []byte {
	return nil
}

func DeSerialize([]byte) (*GasMoney, error) {
	return nil, nil
}
