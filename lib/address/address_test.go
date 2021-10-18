package address

import (
	"math/rand"
	"testing"
	"time"

	"github.com/memoio/go-mefs-v2/lib/crypto"
	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12-381"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/stretchr/testify/assert"
)

func init() {
	rand.Seed(time.Now().Unix())
}

func TestSecp256k1Address(t *testing.T) {
	assert := assert.New(t)

	_, pk, err := crypto.GenerateKey(crypto.Secp256k1)
	assert.NoError(err)

	aByte, err := pk.Raw()
	assert.NoError(err)

	addr, err := NewSecp256k1Address(aByte)
	panic(addr.String())

	assert.NoError(err)
	assert.Equal(types.SECP256K1, addr.Type())

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

	addr, err := NewBLSAddress(pk)
	assert.NoError(err)
	assert.Equal(types.BLS12, addr.Type())

	str, err := encode(addr)
	assert.NoError(err)

	maybe, err := decode(str)
	assert.NoError(err)
	assert.Equal(addr, maybe)

}
