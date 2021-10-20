package address

import (
	"math/rand"
	"testing"
	"time"

	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12-381"
	"github.com/memoio/go-mefs-v2/lib/crypto/signature/secp256k1"
	"github.com/stretchr/testify/assert"
)

func init() {
	rand.Seed(time.Now().Unix())
}

func TestSecp256k1Address(t *testing.T) {
	assert := assert.New(t)

	_, pk, err := secp256k1.GenerateKey()
	assert.NoError(err)

	aByte, err := pk.CompressedByte()
	assert.NoError(err)

	addr, err := NewAddress(aByte)
	panic(addr.String())

	assert.NoError(err)

	str, err := encode(addr)
	assert.NoError(err)

	maybe, err := decode(str)
	assert.NoError(err)
	assert.Equal(addr, maybe)
}

func TestBLSAddress(t *testing.T) {
	assert := assert.New(t)

	sk, err := bls.GenerateKey()
	assert.NoError(err)

	pk, err := bls.PublicKey(sk)
	assert.NoError(err)

	addr, err := NewAddress(pk)
	t.Log(addr.String(), len(addr.String()))

	naddr, err := NewAddressFromString(addr.String())

	t.Logf(naddr.String())

	nnaddr, err := NewAddress([]byte(addr.Raw()))

	t.Logf(nnaddr.String())

	panic(addr.String())
	assert.NoError(err)

	str, err := encode(addr)
	assert.NoError(err)

	maybe, err := decode(str)
	assert.NoError(err)
	assert.Equal(addr, maybe)

}
