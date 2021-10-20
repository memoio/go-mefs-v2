package signature

import (
	"bytes"
	"strconv"
	"testing"

	"github.com/zeebo/blake3"

	"github.com/memoio/go-mefs-v2/lib/crypto/signature/bls"
	sig_common "github.com/memoio/go-mefs-v2/lib/crypto/signature/common"
	"github.com/memoio/go-mefs-v2/lib/crypto/signature/secp256k1"
)

func BenchmarkSign(b *testing.B) {
	sk, pk, err := bls.GenerateKey()
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
	testSign(sig_common.Secp256k1)
	testSign(sig_common.BLS)

	testSign2(sig_common.Secp256k1)
	testSign2(sig_common.BLS)

	testSign3(sig_common.Secp256k1)
	testSign3(sig_common.BLS)

	testSign4(sig_common.Secp256k1)
	testSign4(sig_common.BLS)
}

func testSign(typ sig_common.KeyType) {
	var sk sig_common.PrivKey
	var pk sig_common.PubKey
	var err error

	if typ == sig_common.BLS {
		sk, pk, err = bls.GenerateKey()
		if err != nil {
			panic(err)
		}
	} else if typ == sig_common.Secp256k1 {
		sk, pk, err = secp256k1.GenerateKey()
		if err != nil {
			panic(err)
		}
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

	var newsk sig_common.PrivKey
	var newpk sig_common.PubKey
	if typ == sig_common.BLS {
		newsk = &bls.PrivateKey{}
		err := newsk.Deserialize(skBytes)
		if err != nil {
			panic(err)
		}
		newpk = newsk.GetPublic()
	} else if typ == sig_common.Secp256k1 {
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

	if typ == sig_common.BLS {
		if !bytes.Equal(sig, newsig) {
			panic("sig not equal")
		}
	}
}

func testSign2(typ sig_common.KeyType) {
	var sk sig_common.PrivKey
	var pk sig_common.PubKey
	var err error

	if typ == sig_common.BLS {
		sk, pk, err = bls.GenerateKey()
		if err != nil {
			panic(err)
		}
	} else if typ == sig_common.Secp256k1 {
		sk, pk, err = secp256k1.GenerateKey()
		if err != nil {
			panic(err)
		}
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

	var newsk sig_common.PrivKey
	var newpk sig_common.PubKey
	if typ == sig_common.BLS {
		newsk = &bls.PrivateKey{}
		err := newsk.Deserialize(skBytes)
		if err != nil {
			panic(err)
		}
		newpk = newsk.GetPublic()
	} else if typ == sig_common.Secp256k1 {
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

	if typ == sig_common.BLS {
		if !bytes.Equal(sig, newsig) {
			panic("sig not equal")
		}
	}
}

func testSign3(typ sig_common.KeyType) {
	var sk sig_common.PrivKey
	var pk sig_common.PubKey
	var err error

	if typ == sig_common.BLS {
		sk, pk, err = bls.GenerateKey()
		if err != nil {
			panic(err)
		}
	} else if typ == sig_common.Secp256k1 {
		sk, pk, err = secp256k1.GenerateKey()
		if err != nil {
			panic(err)
		}
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

	var newsk sig_common.PrivKey
	var newpk sig_common.PubKey
	if typ == sig_common.BLS {
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
	} else if typ == sig_common.Secp256k1 {
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

func testSign4(typ sig_common.KeyType) {
	var sk sig_common.PrivKey
	var pk sig_common.PubKey
	var err error

	if typ == sig_common.BLS {
		sk, pk, err = bls.GenerateKey()
		if err != nil {
			panic(err)
		}
	} else if typ == sig_common.Secp256k1 {
		sk, pk, err = secp256k1.GenerateKey()
		if err != nil {
			panic(err)
		}
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

	var newsk sig_common.PrivKey
	var newpk sig_common.PubKey
	if typ == sig_common.BLS {
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
	} else if typ == sig_common.Secp256k1 {
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
