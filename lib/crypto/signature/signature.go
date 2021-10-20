package signature

import (
	"fmt"
	"strconv"

	"github.com/pkg/errors"

	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/crypto/signature/bls"
	sig_common "github.com/memoio/go-mefs-v2/lib/crypto/signature/common"
	"github.com/memoio/go-mefs-v2/lib/crypto/signature/secp256k1"
)

func ParsePrivateKey(privatekey []byte, typ sig_common.KeyType) (sig_common.PrivKey, error) {
	var privkey sig_common.PrivKey
	switch typ {
	case sig_common.BLS:
		privkey = &bls.PrivateKey{}
		err := privkey.Deserialize(privatekey)
		if err != nil {
			return nil, err
		}
	case sig_common.Secp256k1:
		privkey = &secp256k1.PrivateKey{}
		err := privkey.Deserialize(privatekey)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.Wrap(sig_common.ErrBadKeyType, strconv.Itoa(int(typ)))
	}
	return privkey, nil
}

func ParsePubByte(pubbyte []byte) (sig_common.PubKey, error) {
	var pubKey sig_common.PubKey
	plen := len(pubbyte)
	fmt.Println(plen)
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
		return nil, sig_common.ErrBadKeyType
	}
	return pubKey, nil
}

func Verify(addr address.Address, msg, sig []byte) (bool, error) {
	pubkey, err := ParsePubByte(addr.Bytes())
	if err != nil {
		return false, err
	}

	return pubkey.Verify(msg, sig)
}

func GetAdressFromPubkey(pubkey sig_common.PubKey) (address.Address, error) {
	addr, err := pubkey.CompressedByte()
	if err != nil {
		return address.Undef, err
	}
	return address.NewAddress(addr)
}
