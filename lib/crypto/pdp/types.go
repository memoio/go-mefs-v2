package pdp

import (
	"encoding/binary"

	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	pdpv2 "github.com/memoio/go-mefs-v2/lib/crypto/pdp/version2"
)

func GenerateKey(ver uint16) (pdpcommon.KeySet, error) {
	switch ver {
	case pdpcommon.PDPV2:
		return pdpv2.GenKeySet()
	default:
		return nil, pdpcommon.ErrInvalidSettings
	}
}

func GenerateKeyWithSeed(ver uint16, seed []byte) (pdpcommon.KeySet, error) {
	switch ver {
	case pdpcommon.PDPV2:
		return pdpv2.GenKeySetWithSeed(seed, pdpv2.SCount)
	default:
		return nil, pdpcommon.ErrInvalidSettings
	}
}

func DeserializeSecretKey(data []byte) (pdpcommon.SecretKey, error) {
	if len(data) <= 2 {
		return nil, pdpcommon.ErrNumOutOfRange
	}
	v := binary.BigEndian.Uint16(data[:2])
	var sk pdpcommon.SecretKey
	switch v {
	case pdpcommon.PDPV2:
		sk = new(pdpv2.SecretKey)
	default:
		return nil, pdpcommon.ErrInvalidSettings
	}

	err := sk.Deserialize(data)
	if err != nil {
		return nil, err
	}
	return sk, nil
}

func DeserializePublicKey(data []byte) (pdpcommon.PublicKey, error) {
	if len(data) <= 2 {
		return nil, pdpcommon.ErrNumOutOfRange
	}
	v := binary.BigEndian.Uint16(data[:2])
	var pk pdpcommon.PublicKey
	switch v {
	case pdpcommon.PDPV2:
		pk = new(pdpv2.PublicKey)
	default:
		return nil, pdpcommon.ErrInvalidSettings
	}

	err := pk.Deserialize(data)
	if err != nil {
		return nil, err
	}
	return pk, err
}

func DeserializeVerifyKey(data []byte) (pdpcommon.VerifyKey, error) {
	if len(data) <= 2 {
		return nil, pdpcommon.ErrNumOutOfRange
	}
	v := binary.BigEndian.Uint16(data[:2])
	var vk pdpcommon.VerifyKey
	switch v {
	case pdpcommon.PDPV2:
		vk = new(pdpv2.VerifyKey)
	default:
		return nil, pdpcommon.ErrInvalidSettings
	}

	err := vk.Deserialize(data)
	if err != nil {
		return nil, err
	}
	return vk, err
}

func DeserializeProof(data []byte) (pdpcommon.Proof, error) {
	if len(data) <= 2 {
		return nil, pdpcommon.ErrNumOutOfRange
	}
	v := binary.BigEndian.Uint16(data[:2])
	var proof pdpcommon.Proof
	switch v {
	case pdpcommon.PDPV2:
		proof = new(pdpv2.Proof)
	default:
		return nil, pdpcommon.ErrInvalidSettings
	}

	err := proof.Deserialize(data)
	if err != nil {
		return nil, err
	}
	return proof, nil
}

func DeserializeChallenge(data []byte) (pdpcommon.Challenge, error) {
	v := binary.BigEndian.Uint16(data[:2])
	var chal pdpcommon.Challenge
	switch v {
	case pdpcommon.PDPV2:
		chal = new(pdpv2.Challenge)
	default:
		return nil, pdpcommon.ErrInvalidSettings
	}
	err := chal.Deserialize(data)
	if err != nil {
		return nil, err
	}
	return chal, nil
}

func Validate(pk pdpcommon.PublicKey, vk pdpcommon.VerifyKey) bool {
	if pk.Version() != vk.Version() {
		return false
	}
	switch pk.Version() {
	case pdpcommon.PDPV2:
		pk2 := pk.(*pdpv2.PublicKey)
		vk2 := vk.(*pdpv2.VerifyKey)
		return pk2.Validate(vk2)
	default:
		return false
	}
}

func NewChallenge(vk pdpcommon.VerifyKey, r [32]byte) (pdpcommon.Challenge, error) {
	switch vk.Version() {
	case pdpcommon.PDPV2:
		return pdpv2.NewChallenge(r), nil
	default:
		return nil, pdpcommon.ErrInvalidSettings
	}
}

func NewDataVerifier(pk pdpcommon.PublicKey, sk pdpcommon.SecretKey) (pdpcommon.DataVerifier, error) {
	switch pk.Version() {
	case pdpcommon.PDPV2:
		return pdpv2.NewDataVerifier(pk, sk), nil
	default:
		return nil, pdpcommon.ErrInvalidSettings
	}
}

func NewProofAggregator(pk pdpcommon.PublicKey, r [32]byte) (pdpcommon.ProofAggregator, error) {
	switch pk.Version() {
	case pdpcommon.PDPV2:
		return pdpv2.NewProofAggregator(pk, r), nil
	default:
		return nil, pdpcommon.ErrInvalidSettings
	}
}

func CheckTag(ver uint16, tag []byte) error {
	switch ver {
	case pdpcommon.PDPV2:
		return pdpv2.CheckTag(tag)
	default:
		return pdpcommon.ErrInvalidSettings
	}
}
