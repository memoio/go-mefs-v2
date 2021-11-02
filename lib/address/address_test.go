package address

import (
	"math/rand"
	"testing"
	"time"

	"github.com/memoio/go-mefs-v2/lib/crypto/signature"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/stretchr/testify/assert"
)

func init() {
	rand.Seed(time.Now().Unix())
}

func TestAddress(t *testing.T) {
	assert := assert.New(t)

	sk, err := signature.GenerateKey(types.Secp256k1)
	assert.NoError(err)

	aByte, err := sk.GetPublic().Raw()
	assert.NoError(err)

	addr, err := NewAddress(aByte)

	addrs := addr.String()

	naddr, err := NewFromString(addrs)

	res := make([]Address, 0, 1)

	res = append(res, naddr)
	t.Fatal(res[0])
}

func TestSecp256k1Address(t *testing.T) {
	assert := assert.New(t)

	sk, err := signature.GenerateKey(types.Secp256k1)
	assert.NoError(err)

	pk := sk.GetPublic()

	aByte, err := pk.Raw()
	assert.NoError(err)

	addr, err := NewAddress(aByte)
	t.Log(addr.String(), len(addr.String()), len(aByte))
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

	sk, err := signature.GenerateKey(types.BLS)
	assert.NoError(err)

	pk := sk.GetPublic()

	aByte, err := pk.Raw()
	assert.NoError(err)

	addr, err := NewAddress(aByte)
	t.Log(addr.String(), len(addr.String()))

	naddr, err := NewFromString(addr.String())

	t.Logf(naddr.String())

	panic(addr.String())
	assert.NoError(err)

	str, err := encode(addr)
	assert.NoError(err)

	maybe, err := decode(str)
	assert.NoError(err)
	assert.Equal(addr, maybe)

}
