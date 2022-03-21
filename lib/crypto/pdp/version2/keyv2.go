package pdpv2

import (
	"encoding/binary"
	"math/rand"
	"time"

	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12_381"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	"github.com/zeebo/blake3"
	"golang.org/x/xerrors"
)

var _ pdpcommon.KeySet = (*KeySet)(nil)
var _ pdpcommon.SecretKey = (*SecretKey)(nil)
var _ pdpcommon.PublicKey = (*PublicKey)(nil)
var _ pdpcommon.VerifyKey = (*VerifyKey)(nil)
var _ pdpcommon.Challenge = (*Challenge)(nil)
var _ pdpcommon.Proof = (*Proof)(nil)
var _ pdpcommon.DataVerifier = (*DataVerifier)(nil)
var _ pdpcommon.ProofAggregator = (*ProofAggregator)(nil)

const MaxPkNumCount = SCount

// the data structures for the Proof of data possession

// SecretKey is bls secret key
type SecretKey struct {
	BlsSk     Fr
	Alpha     Fr
	ElemAlpha []Fr
}

func (sk *SecretKey) Version() uint16 {
	return pdpcommon.PDPV2
}

func (sk *SecretKey) Serialize() []byte {
	if sk == nil {
		return nil
	}
	buf := make([]byte, 2+2*FrSize)
	binary.BigEndian.PutUint16(buf[:2], sk.Version())
	copy(buf[2:2+FrSize], bls.FrToBytes(&sk.BlsSk))
	copy(buf[2+FrSize:2+2*FrSize], bls.FrToBytes(&sk.Alpha))
	return buf
}

func (sk *SecretKey) Deserialize(data []byte) error {
	if sk == nil {
		return pdpcommon.ErrKeyIsNil
	}
	if len(data) != 2+2*FrSize {
		return pdpcommon.ErrNumOutOfRange
	}
	ver := binary.BigEndian.Uint16(data[:2])
	if ver != pdpcommon.PDPV2 {
		return pdpcommon.ErrVersionUnmatch
	}

	err := bls.FrFromBytes(&sk.BlsSk, data[2:2+FrSize])
	if err != nil {
		return err
	}
	err = bls.FrFromBytes(&sk.Alpha, data[2+FrSize:2+2*FrSize])
	if err != nil {
		return err
	}
	sk.Calculate()
	return nil
}

func (sk *SecretKey) Calculate() {
	if sk == nil {
		return
	}
	if int64(len(sk.ElemAlpha)) != SCount {
		sk.ElemAlpha = make([]Fr, SCount)
	}
	bls.FrSetInt64(&sk.ElemAlpha[0], 1)

	var i int64
	// alpha_i = alpha ^ i
	for i = 1; i < SCount; i++ {
		bls.FrMulMod(&sk.ElemAlpha[i], &sk.ElemAlpha[i-1], &sk.Alpha)
	}
}

// PublicKey is bls public key
type PublicKey struct {
	Count      int64
	BlsPk      G2   // pk = g_2 * sk
	Zeta       G2   // zeta = g_2 * (alpha * sk)
	Phi        G1   // phi = g_1 * sk
	ElemAlphas []G1 // g_1 * alpha^0,g_1 * alpha^1...g_1 * alpha^count-1
}

func (pk *PublicKey) Version() uint16 {
	return pdpcommon.PDPV2
}

func (pk *PublicKey) GetCount() int64 {
	return pk.Count
}

func (pk *PublicKey) VerifyKey() pdpcommon.VerifyKey {
	return &VerifyKey{
		BlsPk: pk.BlsPk,
		Zeta:  pk.Zeta,
		Phi:   pk.Phi,
	}
}

func (pk *PublicKey) Serialize() []byte {
	if pk == nil {
		return nil
	}
	buf := make([]byte, 10+2*G2Size+G1Size+int(pk.Count)*G1Size)

	binary.BigEndian.PutUint16(buf[:2], pk.Version())
	binary.BigEndian.PutUint64(buf[2:10], uint64(pk.Count))
	copy(buf[10:10+G2Size], bls.G2Serialize(&pk.BlsPk))
	copy(buf[10+G2Size:10+2*G2Size], bls.G2Serialize(&pk.Zeta))
	copy(buf[10+2*G2Size:10+2*G2Size+G1Size], bls.G1Serialize(&pk.Phi))

	for i := 0; i < int(pk.Count); i++ {
		copy(buf[10+2*G2Size+G1Size+i*G1Size:10+2*G2Size+G1Size+(i+1)*G1Size], bls.G1Serialize(&pk.ElemAlphas[i]))
	}
	return buf
}

func (pk *PublicKey) Deserialize(data []byte) error {
	if pk == nil {
		return pdpcommon.ErrKeyIsNil
	}
	if len(data) < 10+2*G2Size+G1Size {
		return xerrors.Errorf("length is too short")
	}

	ver := binary.BigEndian.Uint16(data[:2])
	if ver != pdpcommon.PDPV2 {
		return pdpcommon.ErrVersionUnmatch
	}

	pk.Count = int64(binary.BigEndian.Uint64(data[2:10]))
	if pk.Count > MaxPkNumCount {
		return pdpcommon.ErrInvalidSettings
	}
	bls.G2Deserialize(&pk.BlsPk, data[10:10+G2Size])
	bls.G2Deserialize(&pk.Zeta, data[10+G2Size:10+2*G2Size])
	bls.G1Deserialize(&pk.Phi, data[10+2*G2Size:10+2*G2Size+G1Size])
	if (len(data)-(10+2*G2Size+G1Size))/G1Size != int(pk.Count) {
		return pdpcommon.ErrNumOutOfRange
	}
	if int64(len(pk.ElemAlphas)) != pk.Count {
		pk.ElemAlphas = make([]G1, pk.Count)
	}
	for i := 0; i < int(pk.Count); i++ {
		bls.G1Deserialize(&pk.ElemAlphas[i], data[10+2*G2Size+G1Size+i*G1Size:10+2*G2Size+G1Size+(i+1)*G1Size])
	}
	return nil
}

// How to verify G2?
func (k *PublicKey) Validate(vk *VerifyKey) bool {
	if !bls.G2Equal(&k.BlsPk, &vk.BlsPk) ||
		!bls.G2Equal(&k.Zeta, &vk.Zeta) ||
		!bls.G1Equal(&k.Phi, &vk.Phi) {
		return false
	}

	if k == nil || len(k.ElemAlphas) == 0 {
		return false
	}
	if len(k.ElemAlphas) != int(k.Count) {
		return false
	}
	if !bls.G1Equal(&GenG1, &k.ElemAlphas[0]) {
		return false
	}

	for i := int64(0); i < k.Count-1; i++ {
		if !bls.PairingsVerify(&k.ElemAlphas[i], &k.Zeta, &k.ElemAlphas[i+1], &k.BlsPk) {
			return false
		}
	}
	return true
}

func (k *PublicKey) ValidateFast(vk *VerifyKey) bool {
	if !bls.G2Equal(&k.BlsPk, &vk.BlsPk) ||
		!bls.G2Equal(&k.Zeta, &vk.Zeta) ||
		!bls.G1Equal(&k.Phi, &vk.Phi) {
		return false
	}

	if k == nil || len(k.ElemAlphas) == 0 {
		return false
	}
	if len(k.ElemAlphas) != int(k.Count) {
		return false
	}
	if !bls.G1Equal(&GenG1, &k.ElemAlphas[0]) {
		return false
	}

	rand.Seed(time.Now().UnixNano())
	for i := 0; i < 470; i++ {
		index := rand.Int63n(k.Count - 1)
		if !bls.PairingsVerify(&k.ElemAlphas[index], &k.Zeta, &k.ElemAlphas[index+1], &k.BlsPk) {
			return false
		}
	}
	return true
}

type VerifyKey struct {
	BlsPk G2 // pk = g_2 * sk
	Zeta  G2 // zeta = g_2 * (alpha * sk)
	Phi   G1 // phi = g_1 * sk
}

func (vk *VerifyKey) Version() uint16 {
	return pdpcommon.PDPV2
}

func (vk *VerifyKey) Hash() []byte {
	fsIDBytes := blake3.Sum256(vk.Serialize())
	return fsIDBytes[:20]
}

func (vk *VerifyKey) Serialize() []byte {
	if vk == nil {
		return nil
	}
	buf := make([]byte, 2+2*G2Size+G1Size)
	binary.BigEndian.PutUint16(buf[:2], vk.Version())
	copy(buf[2:2+G2Size], bls.G2Serialize(&vk.BlsPk))
	copy(buf[2+G2Size:2+2*G2Size], bls.G2Serialize(&vk.Zeta))
	copy(buf[2+2*G2Size:2+2*G2Size+G1Size], bls.G1Serialize(&vk.Phi))
	return buf
}

func (vk *VerifyKey) Deserialize(data []byte) error {
	if vk == nil {
		return pdpcommon.ErrKeyIsNil
	}
	if len(data) != 2+2*G2Size+G1Size {
		return pdpcommon.ErrNumOutOfRange
	}

	ver := binary.BigEndian.Uint16(data[:2])
	if ver != pdpcommon.PDPV2 {
		return pdpcommon.ErrVersionUnmatch
	}

	bls.G2Deserialize(&vk.BlsPk, data[2:2+G2Size])
	bls.G2Deserialize(&vk.Zeta, data[2+G2Size:2+2*G2Size])
	bls.G1Deserialize(&vk.Phi, data[2+2*G2Size:2+2*G2Size+G1Size])
	return nil
}

// KeySet is wrap
type KeySet struct {
	Pk  *PublicKey
	Sk  *SecretKey
	typ int
}

// GenKeySetWithSeed create instance
func GenKeySetWithSeed(seed []byte, count int64) (*KeySet, error) {
	sk := &SecretKey{
		ElemAlpha: make([]Fr, count),
	}

	pk := &PublicKey{
		Count:      count,
		ElemAlphas: make([]G1, count),
	}

	ks := &KeySet{pk, sk, DefaultType}

	// bls
	// private key
	seed1 := blake3.Sum256(seed)
	bls.FrFromBytes(&sk.BlsSk, seed1[:])

	seed2 := blake3.Sum256(seed1[:])
	bls.FrFromBytes(&sk.Alpha, seed2[:])

	//g_1
	pk.ElemAlphas[0] = GenG1

	ks.Calculate()

	// return instance
	return ks, nil
}

// GenKeySet create instance
func GenKeySet() (*KeySet, error) {
	pk := &PublicKey{
		Count:      SCount,
		ElemAlphas: make([]G1, SCount),
	}
	sk := &SecretKey{
		ElemAlpha: make([]Fr, SCount),
	}
	ks := &KeySet{pk, sk, DefaultType}

	// bls
	// private key
	bls.FrRandom(&sk.BlsSk)
	bls.FrRandom(&sk.Alpha)

	//g_1
	pk.ElemAlphas[0] = GenG1

	ks.Calculate()

	// return instance
	return ks, nil
}

// Calculate cals Xi = x^i, Ui and Wi i = 0, 1, ..., N
func (k *KeySet) Calculate() {
	var oneFr Fr
	bls.FrSetInt64(&oneFr, 1)
	k.Sk.ElemAlpha[0] = oneFr

	var i int64
	// alpha_i = alpha ^ i
	for i = 1; i < k.Pk.Count; i++ {
		bls.FrMulMod(&k.Sk.ElemAlpha[i], &k.Sk.ElemAlpha[i-1], &k.Sk.Alpha)
	}

	var tempFr Fr
	bls.FrMulMod(&tempFr, &k.Sk.Alpha, &k.Sk.BlsSk)

	//zeta = g_2 * (sk*alpha)
	bls.G2Mul(&k.Pk.Zeta, &GenG2, &tempFr)

	//pk = g_2 * sk
	bls.G2Mul(&k.Pk.BlsPk, &GenG2, &k.Sk.BlsSk)

	bls.G1Mul(&k.Pk.Phi, &GenG1, &k.Sk.BlsSk)

	// U = u^(x^i), i = 0, 1, ..., tagCount-1
	for i = 1; i < k.Pk.Count; i++ {
		bls.G1Mul(&k.Pk.ElemAlphas[i], &k.Pk.ElemAlphas[0], &k.Sk.ElemAlpha[i])
	}

}

func (k *KeySet) PublicKey() pdpcommon.PublicKey {
	return k.Pk
}

func (k *KeySet) VerifyKey() pdpcommon.VerifyKey {
	return &VerifyKey{
		BlsPk: k.Pk.BlsPk,
		Zeta:  k.Pk.Zeta,
		Phi:   k.Pk.Phi,
	}
}

func (k *KeySet) SecreteKey() pdpcommon.SecretKey {
	return k.Sk
}

func (k *KeySet) Version() uint16 {
	return pdpcommon.PDPV2
}
