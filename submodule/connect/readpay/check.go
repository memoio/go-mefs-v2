package readpay

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/fxamacker/cbor/v2"
	"github.com/memoio/go-mefs-v2/lib/utils"
	"golang.org/x/crypto/sha3"
	"golang.org/x/xerrors"
)

type Check struct {
	ContractAddr common.Address
	OwnerAddr    common.Address
	ToAddr       common.Address
	Value        *big.Int
	Nonce        uint64
	FromAddr     common.Address
	Sig          []byte
}

func (c *Check) Serialize() ([]byte, error) {
	return cbor.Marshal(c)
}

func (c *Check) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, c)
}

func (c *Check) Hash() []byte {
	buf := make([]byte, 8)
	d := sha3.NewLegacyKeccak256()

	d.Write(c.ContractAddr.Bytes())
	d.Write(c.OwnerAddr.Bytes())
	d.Write(c.ToAddr.Bytes())
	d.Write(utils.LeftPadBytes(c.Value.Bytes(), 32))
	binary.BigEndian.PutUint64(buf, c.Nonce)
	d.Write(buf)
	d.Write(c.FromAddr.Bytes())

	h := d.Sum(nil)

	return h[:]
}

func (c *Check) Equal(c2 *Check) (bool, error) {
	if bytes.Equal(c.Hash(), c2.Hash()) {
		return true, nil
	}

	if c.ContractAddr != c2.ContractAddr {
		return false, xerrors.New("from not equal")
	}
	if c.OwnerAddr != c2.OwnerAddr {
		return false, xerrors.New("from not equal")
	}
	if c.ToAddr != c2.ToAddr {
		return false, xerrors.New("from not equal")
	}
	if c.Value.Cmp(c2.Value) != 0 {
		return false, xerrors.New("value not equal")
	}
	if c.FromAddr != c2.FromAddr {
		return false, xerrors.New("from not equal")
	}
	return true, nil
}

// verify signature of a check
func (c *Check) Verify() (bool, error) {
	// signature to public key
	pubKey, err := crypto.Ecrecover(c.Hash(), c.Sig)
	if err != nil {
		return false, xerrors.Errorf("recover fail: %w", err)
	}

	// pub key to common.address
	recAddr := utils.ToEthAddress(pubKey)

	fmt.Printf("readpay: got %s expect %s", hex.EncodeToString(recAddr), hex.EncodeToString(c.OwnerAddr.Bytes()))

	return bytes.Equal(recAddr, c.OwnerAddr.Bytes()), nil
}

// Paycheck is an auto generated low-level Go binding around an user-defined struct.
type Paycheck struct {
	Check
	PayValue *big.Int
	PaySig   []byte
}

func (p *Paycheck) Hash() []byte {

	buf := make([]byte, 8)
	d := sha3.NewLegacyKeccak256()

	d.Write(p.ContractAddr.Bytes())
	d.Write(p.OwnerAddr.Bytes())
	d.Write(p.ToAddr.Bytes())
	d.Write(utils.LeftPadBytes(p.Value.Bytes(), 32))
	binary.BigEndian.PutUint64(buf, p.Nonce)
	d.Write(buf)
	d.Write(p.FromAddr.Bytes())

	d.Write(utils.LeftPadBytes(p.PayValue.Bytes(), 32))

	h := d.Sum(nil)

	return h[:]
}

func (p *Paycheck) Serialize() ([]byte, error) {
	return cbor.Marshal(p)
}

func (p *Paycheck) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, p)
}

// verify signature of paycheck
func (p *Paycheck) Verify() (bool, error) {
	ok, err := p.Check.Verify()
	if !ok || err != nil {
		return ok, err
	}

	// signature to public key
	pubKey, err := crypto.Ecrecover(p.Hash(), p.PaySig)
	if err != nil {
		return false, xerrors.Errorf("paychecl ecrecover fail: %w", err)
	}

	// pub key to common.address
	recAddr := utils.ToEthAddress(pubKey)

	fmt.Printf("readpay paycheck: got %s expect %s", hex.EncodeToString(recAddr), hex.EncodeToString(p.FromAddr.Bytes()))

	return bytes.Equal(recAddr, p.FromAddr.Bytes()), nil
}
