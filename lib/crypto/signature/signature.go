package signature

import (
	"strconv"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"

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

func GetAdressFromPubkey(pubkey sig_common.PubKey) ([]byte, error) {
	var addr []byte
	var err error
	switch pubkey.Type() {
	case sig_common.BLS:
		addr, err = pubkey.Raw()
		if err != nil {
			return nil, err
		}
	case sig_common.Secp256k1:
		pubBytes, err := pubkey.Raw()
		if err != nil {
			return nil, err
		}
		addr = crypto.Keccak256(pubBytes[1:])[12:]
	default:
		return nil, sig_common.ErrBadKeyType
	}
	return addr, nil
}
