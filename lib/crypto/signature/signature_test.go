package signature

import (
	"bytes"
	"strconv"
	"testing"

	"github.com/zeebo/blake3"

	"github.com/memoio/go-mefs-v2/lib/crypto/signature/bls"
	"github.com/memoio/go-mefs-v2/lib/crypto/signature/common"
	"github.com/memoio/go-mefs-v2/lib/crypto/signature/secp256k1"
	"github.com/memoio/go-mefs-v2/lib/types"
)

func BenchmarkSign(b *testing.B) {
	sk, err := bls.GenerateKey()
	if err != nil {
		panic(err)
	}

	pk := sk.GetPublic()

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
	testSign(types.Secp256k1)
	testSign(types.BLS)

	testSign2(types.Secp256k1)
	testSign2(types.BLS)

	testSign3(types.Secp256k1)
	testSign3(types.BLS)

	testSign4(types.Secp256k1)
	testSign4(types.BLS)
}

func testSign(typ types.KeyType) {
	var sk common.PrivKey
	var pk common.PubKey
	var err error

	if typ == types.BLS {
		sk, err = bls.GenerateKey()
		if err != nil {
			panic(err)
		}

		pk = sk.GetPublic()
	} else if typ == types.Secp256k1 {
		sk, err = secp256k1.GenerateKey()
		if err != nil {
			panic(err)
		}

		pk = sk.GetPublic()
	}

	msg := blake3.Sum512([]byte("1"))

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

	skBytes, err := sk.Raw()
	if err != nil {
		panic(err)
	}

	var newsk common.PrivKey
	var newpk common.PubKey
	if typ == types.BLS {
		newsk = &bls.PrivateKey{}
		err := newsk.Deserialize(skBytes)
		if err != nil {
			panic(err)
		}
		newpk = newsk.GetPublic()
	} else if typ == types.Secp256k1 {
		newsk = &secp256k1.PrivateKey{}
		err := newsk.Deserialize(skBytes)
		if err != nil {
			panic(err)
		}
		newpk = newsk.GetPublic()
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

	if typ == types.BLS {
		if !bytes.Equal(sig, newsig) {
			panic("sig not equal")
		}
	}
}

func testSign2(typ types.KeyType) {
	var sk common.PrivKey
	var pk common.PubKey
	var err error

	if typ == types.BLS {
		sk, err = bls.GenerateKey()
		if err != nil {
			panic(err)
		}

		pk = sk.GetPublic()
	} else if typ == types.Secp256k1 {
		sk, err = secp256k1.GenerateKey()
		if err != nil {
			panic(err)
		}

		pk = sk.GetPublic()
	}

	msg := blake3.Sum256([]byte("1"))

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

	skBytes, err := sk.Raw()
	if err != nil {
		panic(err)
	}

	var newsk common.PrivKey
	var newpk common.PubKey
	if typ == types.BLS {
		newsk = &bls.PrivateKey{}
		err := newsk.Deserialize(skBytes)
		if err != nil {
			panic(err)
		}
		newpk = newsk.GetPublic()
	} else if typ == types.Secp256k1 {
		newsk = &secp256k1.PrivateKey{}
		err := newsk.Deserialize(skBytes)
		if err != nil {
			panic(err)
		}
		newpk = newsk.GetPublic()
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

	if typ == types.BLS {
		if !bytes.Equal(sig, newsig) {
			panic("sig not equal")
		}
	}
}

func testSign3(typ types.KeyType) {
	var sk common.PrivKey
	var pk common.PubKey
	var err error

	if typ == types.BLS {
		sk, err = bls.GenerateKey()
		if err != nil {
			panic(err)
		}
		pk = sk.GetPublic()
	} else if typ == types.Secp256k1 {
		sk, err = secp256k1.GenerateKey()
		if err != nil {
			panic(err)
		}

		pk = sk.GetPublic()
	}

	msg := blake3.Sum256([]byte("1"))

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

	skBytes, err := sk.Raw()
	if err != nil {
		panic(err)
	}

	pkBytes, err := pk.Raw()
	if err != nil {
		panic(err)
	}

	var newsk common.PrivKey
	var newpk common.PubKey
	if typ == types.BLS {
		newsk = &bls.PrivateKey{}
		err := newsk.Deserialize(skBytes)
		if err != nil {
			panic(err)
		}
		newpk = &bls.PublicKey{}
		err = newpk.Deserialize(pkBytes)
		if err != nil {
			panic(err)
		}
	} else if typ == types.Secp256k1 {
		newsk = &secp256k1.PrivateKey{}
		err := newsk.Deserialize(skBytes)
		if err != nil {
			panic(err)
		}
		newpk = &secp256k1.PublicKey{}
		err = newpk.Deserialize(pkBytes)
		if err != nil {
			panic(err)
		}
	}

	newskBytes, err := newsk.Raw()
	if err != nil {
		panic(err)
	}

	newpkBytes, err := newpk.Raw()
	if err != nil {
		panic(err)
	}

	if !bytes.Equal(skBytes, newskBytes) {
		panic("sk is not equal")
	}

	if !bytes.Equal(pkBytes, newpkBytes) {
		panic("pk is not equal")
	}

}

func testSign4(typ types.KeyType) {
	var sk common.PrivKey
	var pk common.PubKey
	var err error

	if typ == types.BLS {
		sk, err = bls.GenerateKey()
		if err != nil {
			panic(err)
		}

		pk = sk.GetPublic()
	} else if typ == types.Secp256k1 {
		sk, err = secp256k1.GenerateKey()
		if err != nil {
			panic(err)
		}

		pk = sk.GetPublic()
	}

	msg := blake3.Sum256([]byte("1"))

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

	skBytes, err := sk.Raw()
	if err != nil {
		panic(err)
	}

	pkBytes, err := pk.Raw()
	if err != nil {
		panic(err)
	}

	var newsk common.PrivKey
	var newpk common.PubKey
	if typ == types.BLS {
		newsk = &bls.PrivateKey{}
		err := newsk.Deserialize(skBytes)
		if err != nil {
			panic(err)
		}
		newpk = &bls.PublicKey{}
		err = newpk.Deserialize(pkBytes)
		if err != nil {
			panic(err)
		}
	} else if typ == types.Secp256k1 {
		newsk = &secp256k1.PrivateKey{}
		err := newsk.Deserialize(skBytes)
		if err != nil {
			panic(err)
		}
		newpk = &secp256k1.PublicKey{}
		err = newpk.Deserialize(pkBytes)
		if err != nil {
			panic(err)
		}
	}

	newskBytes, err := newsk.Raw()
	if err != nil {
		panic(err)
	}

	newpkBytes, err := newpk.Raw()
	if err != nil {
		panic(err)
	}

	if !bytes.Equal(skBytes, newskBytes) {
		panic("sk is not equal")
	}

	if !bytes.Equal(pkBytes, newpkBytes) {
		panic("pk is not equal")
	}

}
