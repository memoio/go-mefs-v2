package pdp

import (
	"encoding/binary"
	"fmt"

	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	pdpv2 "github.com/memoio/go-mefs-v2/lib/crypto/pdp/version2"
)

type SecretKeyWithVersion struct {
	Ver uint16
	Sk  pdpcommon.SecretKey
}

func NewSecretKeyWithVersion(sk pdpcommon.SecretKey) *SecretKeyWithVersion {
	if sk == nil {
		return nil
	}
	return &SecretKeyWithVersion{
		Ver: uint16(sk.Version()),
		Sk:  sk,
	}
}

func (skv *SecretKeyWithVersion) Version() int {
	return int(skv.Ver)
}

func (skv *SecretKeyWithVersion) InternalSecretKey() pdpcommon.SecretKey {
	return skv.Sk
}

func (skv *SecretKeyWithVersion) Serialize() []byte {
	if skv == nil {
		return nil
	}
	lenBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBuf, skv.Ver)
	pf := skv.Sk.Serialize()
	buf := make([]byte, 0, 2+len(pf))
	buf = append(buf, lenBuf[:2]...)
	buf = append(buf, pf...)
	return buf
}

func (skv *SecretKeyWithVersion) Deserialize(data []byte) error {
	if len(data) <= 2 {
		return pdpcommon.ErrNumOutOfRange
	}
	v := binary.BigEndian.Uint16(data[:2])
	var sk pdpcommon.SecretKey
	switch v {
	case pdpcommon.PDPV2:
		sk = new(pdpv2.SecretKey)
	default:
		return pdpcommon.ErrInvalidSettings
	}

	err := sk.Deserialize(data[2:])
	if err != nil {
		return err
	}
	skv.Sk = sk
	skv.Ver = v
	return err
}

type PublicKeyWithVersion struct {
	Ver uint16
	Pk  pdpcommon.PublicKey
}

func NewPublicKeyWithVersion(pk pdpcommon.PublicKey) *PublicKeyWithVersion {
	if pk == nil {
		return nil
	}
	return &PublicKeyWithVersion{
		Ver: uint16(pk.Version()),
		Pk:  pk,
	}
}

func (skv *PublicKeyWithVersion) Version() int {
	return int(skv.Ver)
}

func (pkv *PublicKeyWithVersion) InternalPublicKey() pdpcommon.PublicKey {
	return pkv.Pk
}

func (pkv *PublicKeyWithVersion) Serialize() []byte {
	if pkv == nil {
		return nil
	}
	lenBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBuf, pkv.Ver)
	pf := pkv.Pk.Serialize()
	buf := make([]byte, 0, 2+len(pf))
	buf = append(buf, lenBuf[:2]...)
	buf = append(buf, pf...)
	return buf
}

func (pkv *PublicKeyWithVersion) Deserialize(data []byte) error {
	if len(data) <= 2 {
		return pdpcommon.ErrNumOutOfRange
	}
	v := binary.BigEndian.Uint16(data[:2])
	var pk pdpcommon.PublicKey
	switch v {
	case pdpcommon.PDPV2:
		fmt.Println("Deserialize pk v2")
		pk = new(pdpv2.PublicKey)
	default:
		return pdpcommon.ErrInvalidSettings
	}

	err := pk.Deserialize(data[2:])
	if err != nil {
		return err
	}
	pkv.Pk = pk
	pkv.Ver = v
	return err
}

type VerifyKeyWithVersion struct {
	Ver uint16
	Vk  pdpcommon.VerifyKey
}

func NewVerifyKeyWithVersion(vk pdpcommon.VerifyKey) *VerifyKeyWithVersion {
	if vk == nil {
		return nil
	}
	return &VerifyKeyWithVersion{
		Ver: uint16(vk.Version()),
		Vk:  vk,
	}
}

func (vkv *VerifyKeyWithVersion) Version() int {
	return int(vkv.Ver)
}

func (vkv *VerifyKeyWithVersion) InternalVerifyKey() pdpcommon.VerifyKey {
	return vkv.Vk
}

func (vkv *VerifyKeyWithVersion) Serialize() []byte {
	if vkv == nil {
		return nil
	}
	lenBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBuf, vkv.Ver)
	pf := vkv.Vk.Serialize()
	buf := make([]byte, 0, 2+len(pf))
	buf = append(buf, lenBuf[:2]...)
	buf = append(buf, pf...)
	return buf
}

func (vkv *VerifyKeyWithVersion) Deserialize(data []byte) error {
	if len(data) <= 2 {
		return pdpcommon.ErrNumOutOfRange
	}
	v := binary.BigEndian.Uint16(data[:2])
	var vk pdpcommon.VerifyKey
	switch v {
	case pdpcommon.PDPV2:
		vk = new(pdpv2.VerifyKey)
	default:
		return pdpcommon.ErrInvalidSettings
	}

	err := vk.Deserialize(data[2:])
	if err != nil {
		return err
	}
	vkv.Vk = vk
	vkv.Ver = v
	return err
}

//将proof序列化后，加个版本号
type ProofWithVersion struct {
	Ver   uint16
	Proof pdpcommon.Proof
}

func NewProofWithVersion(proof pdpcommon.Proof) *ProofWithVersion {
	if proof == nil {
		return nil
	}
	return &ProofWithVersion{
		Ver:   uint16(proof.Version()),
		Proof: proof,
	}
}

func (pfv *ProofWithVersion) Version() int {
	return int(pfv.Ver)
}

func (pfv *ProofWithVersion) InternalProof() pdpcommon.Proof {
	return pfv.Proof
}

func (pfv *ProofWithVersion) Serialize() []byte {
	if pfv == nil {
		return nil
	}
	lenBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBuf, pfv.Ver)
	pf := pfv.Proof.Serialize()
	buf := make([]byte, 0, 2+len(pf))
	buf = append(buf, lenBuf[:2]...)
	buf = append(buf, pf...)
	return buf
}

func (pfv *ProofWithVersion) Deserialize(data []byte) error {
	if len(data) <= 2 {
		return pdpcommon.ErrNumOutOfRange
	}
	v := binary.BigEndian.Uint16(data[:2])
	var proof pdpcommon.Proof
	switch v {
	case pdpcommon.PDPV2:
		proof = new(pdpv2.Proof)
	default:
		return pdpcommon.ErrInvalidSettings
	}

	err := proof.Deserialize(data[2:])
	if err != nil {
		return err
	}
	pfv.Proof = proof
	pfv.Ver = v
	return err
}

//将Challenge序列化后，加个版本号
type ChallengeWithVersion struct {
	Ver  uint16
	Chal pdpcommon.Challenge
}

func NewChallengeWithVersion(chal pdpcommon.Challenge) *ChallengeWithVersion {
	if chal == nil {
		return nil
	}
	return &ChallengeWithVersion{
		Ver:  uint16(chal.Version()),
		Chal: chal,
	}
}

func (ch *ChallengeWithVersion) Version() int {
	return int(ch.Ver)
}

func (ch *ChallengeWithVersion) InternalChallenge() pdpcommon.Challenge {
	return ch.Chal
}

func (ch *ChallengeWithVersion) Serialize() []byte {
	if ch == nil {
		return nil
	}
	lenBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBuf, ch.Ver)
	chal := ch.Chal.Serialize()
	buf := make([]byte, 0, 2+len(chal))
	buf = append(buf, lenBuf[:2]...)
	buf = append(buf, chal...)
	return buf
}

func (ch *ChallengeWithVersion) Deserialize(data []byte) error {
	v := binary.BigEndian.Uint16(data[:2])
	var chal pdpcommon.Challenge
	switch v {
	case pdpcommon.PDPV2:
		chal = new(pdpv2.Challenge)
	default:
		return pdpcommon.ErrInvalidSettings
	}
	err := chal.Deserialize(data[2:])
	if err != nil {
		return err
	}
	ch.Chal = chal
	ch.Ver = v
	return err
}

func NewDataVerifier(pk pdpcommon.PublicKey, sk pdpcommon.SecretKey) (pdpcommon.DataVerifier, error) {
	switch pk.Version() {
	case pdpcommon.PDPV2:
		return pdpv2.NewDataVerifier(pk, sk), nil
	default:
		return nil, pdpcommon.ErrInvalidSettings
	}
}

func NewProofVerifier(vk pdpcommon.VerifyKey) (pdpcommon.ProofVerifier, error) {
	switch vk.Version() {
	case pdpcommon.PDPV2:
		return pdpv2.NewProofVerifier(vk), nil
	default:
		return nil, pdpcommon.ErrInvalidSettings
	}
}

func NewProofAggregator(pk pdpcommon.PublicKey, r int64) (pdpcommon.ProofAggregator, error) {
	switch pk.Version() {
	case pdpcommon.PDPV2:
		return pdpv2.NewProofAggregator(pk, r), nil
	default:
		return nil, pdpcommon.ErrInvalidSettings
	}
}
