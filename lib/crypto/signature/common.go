package signature

import (
	"bytes"
	"strconv"

	"github.com/pkg/errors"
	"github.com/zeebo/blake3"

	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/crypto/signature/bls"
	"github.com/memoio/go-mefs-v2/lib/crypto/signature/common"
	"github.com/memoio/go-mefs-v2/lib/crypto/signature/secp256k1"
	"github.com/memoio/go-mefs-v2/lib/types"
)

func GenerateKey(typ types.KeyType) (common.PrivKey, error) {
	switch typ {
	case types.BLS:
		return bls.GenerateKey()
	case types.Secp256k1:
		return secp256k1.GenerateKey()
	default:
		return nil, common.ErrBadKeyType
	}
}

func ParsePrivateKey(privatekey []byte, typ types.KeyType) (common.PrivKey, error) {
	var privkey common.PrivKey
	switch typ {
	case types.BLS:
		privkey = &bls.PrivateKey{}
		err := privkey.Deserialize(privatekey)
		if err != nil {
			return nil, err
		}
	case types.Secp256k1:
		privkey = &secp256k1.PrivateKey{}
		err := privkey.Deserialize(privatekey)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.Wrap(common.ErrBadKeyType, strconv.Itoa(int(typ)))
	}
	return privkey, nil
}

// verify related

func ParsePubByte(pubbyte []byte) (common.PubKey, error) {
	var pubKey common.PubKey
	plen := len(pubbyte)
	switch plen {
	case 33, 65:
		pubKey = &secp256k1.PublicKey{}
		err := pubKey.Deserialize(pubbyte)
		if err != nil {
			return nil, err
		}
	case 48:
		pubKey = &bls.PublicKey{}
		err := pubKey.Deserialize(pubbyte)
		if err != nil {
			return nil, err
		}
	default:
		return nil, common.ErrBadKeyType
	}
	return pubKey, nil
}

func Verify(pubBytes []byte, data, sig []byte) (bool, error) {
	plen := len(pubBytes)

	switch plen {
	case 20:
		// for eth address
		msg := blake3.Sum256(data)

		rePub, err := secp256k1.EcRecover(msg[:], sig)
		if err != nil {
			return false, err
		}

		eaddr, err := address.ToEthAddress(rePub)
		if err != nil {
			return false, err
		}

		if bytes.Equal(pubBytes, eaddr) {
			return true, nil
		}
	default:
		pk, err := ParsePubByte(pubBytes)
		if err != nil {
			return false, err
		}

		return pk.Verify(data, sig)
	}
	return false, nil
}
