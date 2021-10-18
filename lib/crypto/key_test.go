package crypto

import (
	"bytes"
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/minio/blake2b-simd"
	"github.com/zeebo/blake3"
)

func init() {
	rand.Seed(time.Now().Unix())
}

func BenchmarkSign(b *testing.B) {
	sk, pk, err := GenerateKeyForAll(BLS)
	if err != nil {
		panic(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg := blake3.Sum256([]byte(strconv.Itoa(i)))
		sig, err := sk.Sign(msg[:])
		if err != nil {
			panic(err)
		}

		ok, err := pk.Verify(msg[:], sig)
		if err != nil {
			panic(err)
		}

		if !ok {
			panic("sign verify wrong")
		}
	}
}

func TestSecpSign(t *testing.T) {
	testSign(Secp256k1)
	testSign(BLS)

	testSign2(Secp256k1)
	testSign2(BLS)

	testSign3(Secp256k1)
	testSign3(BLS)

	testSign4(Secp256k1)
	testSign4(BLS)
}

func testSign(typ KeyType) {
	sk, pk, err := GenerateKey(typ)
	if err != nil {
		panic(err)
	}

	msg := blake2b.Sum256([]byte("1"))

	sig, err := sk.Sign(msg[:])
	if err != nil {
		panic(err)
	}

	ok, err := pk.Verify(msg[:], sig)
	if err != nil {
		panic(err)
	}

	if !ok {
		panic("sign verify wrong")
	}

	skByte, err := Serialize(sk)
	if err != nil {
		panic(err)
	}

	newsk, newpk, err := Unmarshal(skByte)
	if err != nil {
		panic(err)
	}

	newsig, err := newsk.Sign(msg[:])
	if err != nil {
		panic(err)
	}

	newok, err := newpk.Verify(msg[:], newsig)
	if err != nil {
		panic(err)
	}

	if !newok {
		panic("sign verify wrong")
	}

	eqok := sk.Equals(newsk)
	if !eqok {
		panic("secret key not equal")
	}

	eqpok := pk.Equals(newpk)
	if !eqpok {
		panic("pubkey not equal")
	}

	if typ == BLS {
		if !bytes.Equal(sig, newsig) {
			panic("sig not equal")
		}
	}
}

func testSign2(typ KeyType) {
	sk, pk, err := GenerateKeyForAll(typ)
	if err != nil {
		panic(err)
	}

	msg := blake2b.Sum256([]byte("1"))

	sig, err := sk.Sign(msg[:])
	if err != nil {
		panic(err)
	}

	ok, err := pk.Verify(msg[:], sig)
	if err != nil {
		panic(err)
	}

	if !ok {
		panic("sign verify wrong")
	}

	skByte, err := Marshal(sk)
	if err != nil {
		panic(err)
	}

	newsk, newpk, err := Deserialize(skByte)
	if err != nil {
		panic(err)
	}

	newsig, err := newsk.Sign(msg[:])
	if err != nil {
		panic(err)
	}

	newok, err := newpk.Verify(msg[:], newsig)
	if err != nil {
		panic(err)
	}

	if !newok {
		panic("sign verify wrong")
	}

	eqok := sk.Equals(newsk)
	if !eqok {
		panic("secret key not equal")
	}

	eqpok := pk.Equals(newpk)
	if !eqpok {
		panic("pubkey not equal")
	}

	if typ == BLS {
		if !bytes.Equal(sig, newsig) {
			panic("sig not equal")
		}
	}
}

func testSign3(typ KeyType) {
	sk, pk, err := GenerateKey(typ)
	if err != nil {
		panic(err)
	}

	msg := blake2b.Sum256([]byte("1"))

	sig, err := sk.Sign(msg[:])
	if err != nil {
		panic(err)
	}

	ok, err := pk.Verify(msg[:], sig)
	if err != nil {
		panic(err)
	}

	if !ok {
		panic("sign verify wrong")
	}

	skByte, err := Serialize(sk)
	if err != nil {
		panic(err)
	}

	pkByte, err := Serialize(pk)
	if err != nil {
		panic(err)
	}

	newsk, newpk, err := Deserialize(skByte)
	if err != nil {
		panic(err)
	}

	newskByte, err := Marshal(newsk)
	if err != nil {
		panic(err)
	}

	newpkByte, err := Marshal(newpk)
	if err != nil {
		panic(err)
	}

	if !bytes.Equal(skByte, newskByte) {
		panic("sk is not equal")
	}

	if !bytes.Equal(pkByte, newpkByte) {
		panic("pk is not equal")
	}

}

func testSign4(typ KeyType) {
	sk, pk, err := GenerateKeyForAll(typ)
	if err != nil {
		panic(err)
	}

	msg := blake2b.Sum256([]byte("1"))

	sig, err := sk.Sign(msg[:])
	if err != nil {
		panic(err)
	}

	ok, err := pk.Verify(msg[:], sig)
	if err != nil {
		panic(err)
	}

	if !ok {
		panic("sign verify wrong")
	}

	skByte, err := Marshal(sk)
	if err != nil {
		panic(err)
	}

	pkByte, err := Marshal(pk)
	if err != nil {
		panic(err)
	}

	newsk, newpk, err := Unmarshal(skByte)
	if err != nil {
		panic(err)
	}

	newskByte, err := Serialize(newsk)
	if err != nil {
		panic(err)
	}

	newpkByte, err := Serialize(newpk)
	if err != nil {
		panic(err)
	}

	if !bytes.Equal(skByte, newskByte) {
		panic("sk is not equal")
	}

	if !bytes.Equal(pkByte, newpkByte) {
		panic("pk is not equal")
	}

}
