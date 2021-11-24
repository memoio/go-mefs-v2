package address

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/memoio/go-mefs-v2/lib/crypto/signature"
	"github.com/memoio/go-mefs-v2/lib/types"
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
	assert.NoError(err)

	addrs := addr.String()

	naddr, err := NewFromString(addrs)
	assert.NoError(err)

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

	assert.NoError(err)

	str, err := encode(addr)
	assert.NoError(err)

	maybe, err := decode(str)
	assert.NoError(err)
	assert.Equal(addr, maybe)

	panic(addr.String())
}

func TestBLSAddress(t *testing.T) {
	assert := assert.New(t)

	sk, err := signature.GenerateKey(types.BLS)
	assert.NoError(err)

	pk := sk.GetPublic()

	aByte, err := pk.Raw()
	assert.NoError(err)

	addr, err := NewAddress(aByte)
	assert.NoError(err)
	t.Log(addr.String(), len(addr.String()))

	naddr, err := NewFromString(addr.String())

	t.Logf(naddr.String())

	assert.NoError(err)

	str, err := encode(addr)
	assert.NoError(err)

	maybe, err := decode(str)
	assert.NoError(err)
	assert.Equal(addr, maybe)

	panic(addr.String())
}
