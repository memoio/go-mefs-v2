package readpay

import (
	"crypto/ecdsa"
	"math/big"
	callconts "memoc/callcontracts"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/types"
	"golang.org/x/xerrors"
)

// for test

var (
	// a local account, with enough money in it
	opSk         = "503f38a9c967ed597e47fe25643985f032b072db8075426a92110f82df48dfcb"
	contractAddr = callconts.RoleAddr
	opAddr       = callconts.RoleAddr
)

func init() {
	opAddr, _ = skToAddr(opSk)
	contractAddr = opAddr
}

func skToAddr(sk string) (common.Address, error) {
	skECDSA, err := crypto.HexToECDSA(sk)
	if err != nil {
		return common.Address{}, xerrors.Errorf("convert to ECDSA err: %w", err)
	}

	pubKey := skECDSA.Public()
	pubKeyECDSA, ok := pubKey.(*ecdsa.PublicKey)
	if !ok {
		return common.Address{}, xerrors.New("error casting public key to ECDSA")
	}

	addr := crypto.PubkeyToAddress(*pubKeyECDSA)

	return addr, nil
}

func generateCheck(fromAddr, toAddr common.Address, nonce uint64) (*Check, error) {
	c := &Check{
		ContractAddr: contractAddr,
		OwnerAddr:    opAddr,
		ToAddr:       toAddr,
		Nonce:        nonce,
		Value:        big.NewInt(types.DefaultReadPrice * build.DefaultSegSize * 1024 * 40),
		FromAddr:     fromAddr,
	}

	skECDSA, err := crypto.HexToECDSA(opSk)
	if err != nil {
		return nil, xerrors.Errorf("convert to ECDSA err: %w", err)
	}

	sigByte, err := crypto.Sign(c.Hash(), skECDSA)
	if err != nil {
		return nil, xerrors.Errorf("sign paycheck error: %w", err)
	}
	c.Sig = sigByte

	return c, nil
}
