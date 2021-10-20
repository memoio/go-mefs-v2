package signature

import (
	"encoding/json"
	"fmt"

	"github.com/awnumar/memguard"
	sig_common "github.com/memoio/go-mefs-v2/lib/crypto/signature/common"
	logging "github.com/memoio/go-mefs-v2/lib/log"
)

// todo

var log = logging.Logger("signature")

func init() {
	// Safely terminate in case of an interrupt signal
	memguard.CatchInterrupt()
}

// KeyInfo is a key and its type used for signing.
type KeyInfo struct {
	// Private key.
	PrivateKey *memguard.Enclave `json:"privateKey"`
	// Cryptographic system used to generate private key.
	SigType sig_common.KeyType ` json:"type"`
}

type keyInfo struct {
	// Private key.
	PrivateKey []byte `json:"privateKey"`
	// Cryptographic system used to generate private key.
	SigType byte `json:"type"`
}

func (ki *KeyInfo) UnmarshalJSON(data []byte) error {
	k := keyInfo{}
	err := json.Unmarshal(data, &k)
	if err != nil {
		return err
	}

	ki.SigType = k.SigType

	ki.SetPrivateKey(k.PrivateKey)

	return nil
}

func (ki KeyInfo) MarshalJSON() ([]byte, error) {
	var err error
	var b []byte
	err = ki.UsePrivateKey(func(privateKey []byte) error {
		k := keyInfo{}
		k.PrivateKey = privateKey
		if ki.SigType == sig_common.BLS {
			k.SigType = sig_common.BLS
		} else if ki.SigType == sig_common.Secp256k1 {
			k.SigType = sig_common.Secp256k1
		} else {
			return fmt.Errorf("unsupport keystore types  %T", k.SigType)
		}
		b, err = json.Marshal(k)
		return err
	})

	return b, err
}

// Key returns the private key of KeyInfo
// This method makes the key escape from memguard's protection, so use caution
func (ki *KeyInfo) Key() []byte {
	var pk []byte
	err := ki.UsePrivateKey(func(privateKey []byte) error {
		pk = make([]byte, len(privateKey))
		copy(pk, privateKey[:])
		return nil
	})
	if err != nil {
		log.Errorf("got private key failed %v", err)
		return []byte{}
	}
	return pk
}

func (ki *KeyInfo) UsePrivateKey(f func([]byte) error) error {
	buf, err := ki.PrivateKey.Open()
	if err != nil {
		return err
	}
	defer buf.Destroy()

	return f(buf.Bytes())
}

func (ki *KeyInfo) SetPrivateKey(privateKey []byte) {
	// will wipes privateKey with zeroes
	ki.PrivateKey = memguard.NewEnclave(privateKey)
}
