package pdpv2

import (
	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12_381"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	"github.com/zeebo/blake3"
)

// GenTag create tag for *SINGLE* segment
// typ: 32B atom or 24B atom
// mode: sign or not
func (k *KeySet) GenTag(index []byte, segment []byte, start int, mode bool) ([]byte, error) {
	if k == nil || k.Pk == nil {
		return nil, pdpcommon.ErrKeyIsNil
	}

	atoms, err := splitSegmentToAtoms(segment, k.typ)
	if err != nil {
		return nil, err
	}

	var uMiDel G1
	// Prod(alpha_j^M_ij)，即Prod(g_1^Sigma(alpha^j*M_ij))
	if k.Sk != nil {
		var power Fr
		if start == 0 {
			//FrEvaluatePolynomial
			bls.EvalPolyAt(&power, atoms, &(k.Sk.Alpha))
		} else {
			bls.FrClear(&power) // Set0
			for j, atom := range atoms {
				var mid Fr
				i := j + start
				bls.FrMulMod(&mid, &(k.Sk.ElemAlpha[i]), &atom) // Xi * Mi
				bls.FrAddMod(&power, &power, &mid)              // power = Sigma(Xi*Mi)
			}
		}
		bls.G1Mul(&uMiDel, &(GenG1), &power) // uMiDel = u ^ Sigma(Xi*Mi)
	} else {
		if start == 0 {
			bls.G1MulVec(&uMiDel, k.Pk.ElemAlphas[:len(atoms)], atoms)
		} else {
			for j, atom := range atoms {
				// var Mi Fr
				var mid G1
				i := j + start
				bls.G1Mul(&mid, &k.Pk.ElemAlphas[i], &atom) // uMiDel = ui ^ Mi)
				bls.G1Add(&uMiDel, &uMiDel, &mid)
			}
		}
	}
	if start == 0 {
		// H(Wi)
		var HWi Fr
		h := blake3.Sum256(index)
		bls.FrFromBytes(&HWi, h[:])
		var HWiG1 G1
		bls.G1Mul(&HWiG1, &GenG1, &HWi)

		// r = HWi * (u^Sgima(Xi*Mi))
		bls.G1Add(&uMiDel, &HWiG1, &uMiDel)
	}

	// sign
	if mode {
		// tag = (HWi * (u^Sgima(Xi*Mi))) ^ blsSK
		bls.G1Mul(&uMiDel, &uMiDel, &(k.Sk.BlsSk))
	}

	return bls.G1Serialize(&uMiDel), nil
}

// VerifyTag check segment和tag是否对应
func (k *PublicKey) VerifyTag(index, segment, tag []byte, typ int) (bool, error) {
	if k == nil {
		return false, pdpcommon.ErrKeyIsNil
	}

	var HWiG1, mid, formula, delta G1
	var HWi Fr
	bls.G1Clear(&formula)

	//H(W_i) * g_1
	h := blake3.Sum256(index)
	bls.FrFromBytes(&HWi, h[:])
	bls.G1Mul(&HWiG1, &GenG1, &HWi)

	err := bls.G1Deserialize(&delta, tag)
	if err != nil {
		return false, err
	}

	if bls.G1IsZero(&delta) {
		return false, pdpcommon.ErrDeserializeFailed
	}

	atoms, err := splitSegmentToAtoms(segment, typ)
	if err != nil {
		return false, err
	}

	bls.G1MulVec(&mid, k.ElemAlphas[:len(atoms)], atoms)

	bls.G1Add(&formula, &HWiG1, &mid) // formula = H(Wi) * Prod(uj^mij)

	ok := bls.PairingsVerify(&delta, &GenG2, &formula, &(k.BlsPk)) // left = e(tag, g), right = e(H(Wi) * Prod(uj^mij), pk)

	return ok, nil
}

// GenProof gens
func (pk *PublicKey) GenProof(chal pdpcommon.Challenge, segments, tags [][]byte, typ int) (pdpcommon.Proof, error) {
	if pk == nil || typ <= 0 {
		return nil, pdpcommon.ErrKeyIsNil
	}
	// sums_j为待挑战的各segments位于同一位置(即j)上的atom的和
	if len(segments) == 0 || len(segments[0]) == 0 {
		return nil, pdpcommon.ErrSegmentSize
	}

	tagNum := len(segments[0])
	for _, segment := range segments {
		if len(segment) > tagNum {
			tagNum = len(segment)
		}
	}

	tagNum = tagNum / typ
	sums := make([]Fr, tagNum)
	for _, segment := range segments {
		atoms, err := splitSegmentToAtoms(segment, typ)
		if err != nil {
			return nil, err
		}

		for j, atom := range atoms { // 扫描各segment
			bls.FrAddMod(&sums[j], &sums[j], &atom)
		}
	}
	if len(pk.ElemAlphas) < tagNum {
		return nil, pdpcommon.ErrNumOutOfRange
	}
	var fr_r, pk_r Fr //P_k(r)
	bls.FrSetInt64(&fr_r, chal.Random())
	bls.EvalPolyAt(&pk_r, sums, &fr_r)

	// poly(x) - poly(r) always divides (x - r) since the latter is a root of the former.
	// With that infomation we can jump into the division.
	quotient := make([]Fr, len(sums)-1)
	if len(quotient) > 0 {
		// q_(n - 1) = p_n
		quotient[len(quotient)-1] = sums[len(quotient)]
		for j := len(quotient) - 2; j >= 0; j-- {
			// q_j = p_(j + 1) + q_(j + 1) * r
			bls.FrMulMod(&quotient[j], &quotient[j+1], &fr_r)
			bls.FrAddMod(&quotient[j], &quotient[j], &sums[j+1])
		}
	}
	var psi G1

	bls.G1MulVec(&psi, pk.ElemAlphas[:len(quotient)], quotient)

	// delta = Prod(tag_i)
	var delta G1
	bls.G1Clear(&delta)
	var t G1
	for _, tag := range tags {
		err := bls.G1Deserialize(&t, tag)
		if err != nil {
			return nil, err
		}
		bls.G1Add(&delta, &delta, &t)
	}

	var kappa G1
	bls.G1Mul(&t, &pk.Phi, &pk_r)
	bls.G1Sub(&kappa, &delta, &t)

	return &Proof{
			Psi:   bls.G1Serialize(&psi),
			Kappa: bls.G1Serialize(&kappa),
		},
		nil
}

// VerifyProof verify proof
func (vk *VerifyKey) VerifyProof(chal pdpcommon.Challenge, proof pdpcommon.Proof) (bool, error) {
	if vk == nil {
		return false, pdpcommon.ErrKeyIsNil
	}

	pf, ok := proof.(*Proof)
	if !ok {
		return false, pdpcommon.ErrVersionUnmatch
	}
	var psi, kappa G1

	err := bls.G1Deserialize(&psi, pf.Psi)
	if err != nil {
		return false, err
	}

	if bls.G1IsZero(&psi) {
		return false, nil
	}

	err = bls.G1Deserialize(&kappa, pf.Kappa)
	if err != nil {
		return false, err
	}

	if bls.G1IsZero(&kappa) {
		return false, nil
	}

	var ProdHWi G1
	var G1temp1 G1
	var G2temp1 G2
	var tempFr, HWi Fr
	//var lhs1, lhs2, rhs1 GT

	bls.FrClear(&HWi)
	indices := chal.Indices()
	for _, index := range indices {
		h := blake3.Sum256(index)
		bls.FrFromBytes(&tempFr, h[:])
		bls.FrAddMod(&HWi, &HWi, &tempFr)
	}

	bls.G1Mul(&ProdHWi, &vk.Phi, &HWi)
	bls.G1Sub(&G1temp1, &kappa, &ProdHWi)

	bls.FrSetInt64(&tempFr, chal.Random())
	bls.G2Mul(&G2temp1, &vk.BlsPk, &tempFr)
	bls.G2Sub(&G2temp1, &vk.Zeta, &G2temp1)

	res := bls.PairingsVerify(
		&G1temp1, &GenG2, &psi, &G2temp1,
	)
	return res, nil
	//return bls.GTEqual(&lhs1, &rhs1), nil
}

// VerifyData User or Provider用于聚合验证数据完整性
func (k *KeySet) VerifyData(indices [][]byte, segments, tags [][]byte) (bool, error) {
	if (len(indices) != len(segments)) || (len(indices) != len(tags)) {
		return false, pdpcommon.ErrNumOutOfRange
	}
	if k.Pk == nil {
		return false, pdpcommon.ErrKeyIsNil
	}

	if len(segments) == 0 || len(segments[0]) == 0 {
		return false, pdpcommon.ErrSegmentSize
	}

	tagNum := len(segments[0]) / k.typ
	// sums_j为待挑战的各segments位于同一位置(即j)上的atom的和
	sums := make([]Fr, tagNum)
	for _, segment := range segments {
		atoms, err := splitSegmentToAtoms(segment, k.typ)
		if err != nil {
			return false, err
		}

		for j, atom := range atoms { // 扫描各segment
			if len(atoms) < tagNum {
				return false, pdpcommon.ErrNumOutOfRange
			}
			bls.FrAddMod(&sums[j], &sums[j], &atom)
		}
	}

	if len(k.Pk.ElemAlphas) < tagNum {
		return false, pdpcommon.ErrNumOutOfRange
	}

	// muProd = Prod(u_j^sums_j)
	var muProd G1
	bls.G1Clear(&muProd)
	if k.Sk != nil {
		var power Fr
		bls.EvalPolyAt(&power, sums, &(k.Sk.Alpha))
		bls.G1Mul(&muProd, &(GenG1), &power)
	} else {
		bls.G1MulVec(&muProd, k.Pk.ElemAlphas, sums)
	}
	// delta = Prod(tag_i)
	var delta G1
	bls.G1Clear(&delta)
	for _, tag := range tags {
		var t G1
		err := bls.G1Deserialize(&t, tag)
		if err != nil {
			return false, err
		}

		bls.G1Add(&delta, &delta, &t)
	}

	if bls.G1IsZero(&delta) {
		return false, nil
	}

	var ProdHWi, ProdHWimu G1

	var tempFr, HWi Fr
	bls.G1Clear(&ProdHWi)
	for _, index := range indices {
		h := blake3.Sum256(index)
		bls.FrFromBytes(&tempFr, h[:])
		bls.FrAddMod(&HWi, &HWi, &tempFr)
	}
	bls.G1Mul(&ProdHWi, &GenG1, &HWi)
	bls.G1Add(&ProdHWimu, &ProdHWi, &muProd)

	// var index string
	// var offset int
	// 验证tag和mu是对应的
	// lhs = e(delta, g)
	// rhs = e(Prod(H(Wi)) * mu, pk)
	// check
	if !bls.PairingsVerify(&delta, &GenG2, &ProdHWimu, &k.Pk.BlsPk) {
		return false, pdpcommon.ErrVerifyStepOne
	}

	return true, nil
}

type ProofAggregator struct {
	pk     *PublicKey
	r      int64
	typ    int
	sums   []Fr
	delta  G1
	tempG1 G1
}

func NewProofAggregator(pki pdpcommon.PublicKey, r int64, typ int) pdpcommon.ProofAggregator {
	pk, ok := pki.(*PublicKey)
	if !ok {
		return nil
	}
	sums := make([]Fr, pk.Count)
	var delta G1
	bls.G1Clear(&delta)
	var tempG1 G1
	return &ProofAggregator{pk, r, typ, sums, delta, tempG1}
}

func (pa *ProofAggregator) Version() int {
	return 2
}

func (pa *ProofAggregator) Input(segment []byte, tag []byte) error {
	atoms, err := splitSegmentToAtoms(segment, pa.typ)
	if err != nil {
		return err
	}

	for j, atom := range atoms { // 扫描各segment
		bls.FrAddMod(&pa.sums[j], &pa.sums[j], &atom)
	}

	err = bls.G1Deserialize(&pa.tempG1, tag)
	if err != nil {
		return err
	}
	bls.G1Add(&pa.delta, &pa.delta, &pa.tempG1)
	return nil
}

func (pa *ProofAggregator) InputMulti(segments [][]byte, tags [][]byte) error {
	if len(tags) != len(segments) || len(segments) < 0 {
		return pdpcommon.ErrNumOutOfRange
	}
	le := len(segments[0])
	for i, segment := range segments {
		if len(segment) != le {
			return pdpcommon.ErrNumOutOfRange
		}
		atoms, err := splitSegmentToAtoms(segment, pa.typ)
		if err != nil {
			return err
		}

		for j, atom := range atoms { // 扫描各segment
			bls.FrAddMod(&pa.sums[j], &pa.sums[j], &atom)
		}
		//tag aggregation
		err = bls.G1Deserialize(&pa.tempG1, tags[i])
		if err != nil {
			return err
		}
		bls.G1Add(&pa.delta, &pa.delta, &pa.tempG1)
	}

	return nil
}

func (pa *ProofAggregator) Result() (pdpcommon.Proof, error) {
	var pk_r Fr //P_k(r)
	var fr_r Fr
	bls.FrSetInt64(&fr_r, pa.r)
	bls.EvalPolyAt(&pk_r, pa.sums, &fr_r)

	// poly(x) - poly(r) always divides (x - r) since the latter is a root of the former.
	// With that infomation we can jump into the division.
	quotient := make([]Fr, len(pa.sums)-1)
	if len(quotient) > 0 {
		// q_(n - 1) = p_n
		quotient[len(quotient)-1] = pa.sums[len(quotient)]
		for j := len(quotient) - 2; j >= 0; j-- {
			// q_j = p_(j + 1) + q_(j + 1) * r
			bls.FrMulMod(&quotient[j], &quotient[j+1], &fr_r)
			bls.FrAddMod(&quotient[j], &quotient[j], &pa.sums[j+1])
		}
	}

	var psi G1
	bls.G1MulVec(&psi, pa.pk.ElemAlphas[:len(quotient)], quotient)

	var kappa G1
	bls.G1Mul(&kappa, &pa.pk.Phi, &pk_r)
	bls.G1Sub(&kappa, &pa.delta, &kappa)

	return &Proof{
		Psi:   bls.G1Serialize(&psi),
		Kappa: bls.G1Serialize(&kappa),
	}, nil
}

type DataVerifier struct {
	pk    *PublicKey
	sk    *SecretKey
	typ   int
	sums  []Fr
	delta G1
	HWi   Fr
}

func NewDataVerifier(pki pdpcommon.PublicKey, ski pdpcommon.SecretKey) pdpcommon.DataVerifier {
	pk, ok := pki.(*PublicKey)
	if !ok {
		return nil
	}

	var sk *SecretKey
	if ski != nil {
		sk, ok = ski.(*SecretKey)
		if !ok {
			return nil
		}
	}

	sums := make([]Fr, pk.Count)
	var delta G1
	bls.G1Clear(&delta)
	var HWi Fr
	bls.FrClear(&HWi)

	return &DataVerifier{pk, sk, DefaultType, sums, delta, HWi}
}

func (dv *DataVerifier) Version() int {
	return 2
}

func (dv *DataVerifier) Input(index, segment, tag []byte) error {
	var tempG1 G1
	err := bls.G1Deserialize(&tempG1, tag)
	if err != nil {
		return err
	}

	atoms, err := splitSegmentToAtoms(segment, dv.typ)
	if err != nil {
		return err
	}

	for j, atom := range atoms { // 扫描各segment
		bls.FrAddMod(&dv.sums[j], &dv.sums[j], &atom)
	}

	bls.G1Add(&dv.delta, &dv.delta, &tempG1)

	var tempFr Fr
	h := blake3.Sum256(index)
	bls.FrFromBytes(&tempFr, h[:])
	bls.FrAddMod(&dv.HWi, &dv.HWi, &tempFr)

	return nil
}

func (dv *DataVerifier) InputMulti(indices, segments, tags [][]byte) error {
	if len(segments) <= 0 || len(tags) != len(segments) || len(indices) != len(segments) {
		return pdpcommon.ErrNumOutOfRange
	}
	le := len(segments[0])
	var tempFr Fr
	var tempG1 G1
	for i, segment := range segments {
		if len(segment) != le {
			return pdpcommon.ErrNumOutOfRange
		}

		//tag aggregation
		err := bls.G1Deserialize(&tempG1, tags[i])
		if err != nil {
			return err
		}

		atoms, err := splitSegmentToAtoms(segment, dv.typ)
		if err != nil {
			return err
		}

		bls.G1Add(&dv.delta, &dv.delta, &tempG1)

		for j, atom := range atoms { // 扫描各segment
			bls.FrAddMod(&dv.sums[j], &dv.sums[j], &atom)
		}

		h := blake3.Sum256(indices[i])
		bls.FrFromBytes(&tempFr, h[:])
		bls.FrAddMod(&dv.HWi, &dv.HWi, &tempFr)
	}

	return nil
}

func (dv *DataVerifier) Result() bool {
	var muProd G1
	bls.G1Clear(&muProd)
	if dv.sk != nil {
		var power Fr
		bls.EvalPolyAt(&power, dv.sums, &(dv.sk.Alpha))
		bls.G1Mul(&muProd, &(GenG1), &power)
	} else {
		bls.G1MulVec(&muProd, dv.pk.ElemAlphas, dv.sums)
	}

	if bls.G1IsZero(&dv.delta) {
		return false
	}

	var ProdHWimu G1
	// var index string
	// var offset int
	// 第一步：验证tag和mu是对应的
	// lhs = e(delta, g)

	bls.G1Mul(&ProdHWimu, &GenG1, &dv.HWi)
	bls.G1Add(&ProdHWimu, &ProdHWimu, &muProd)

	return bls.PairingsVerify(&dv.delta, &GenG2, &ProdHWimu, &dv.pk.BlsPk)
}

func (dv *DataVerifier) Reset() {
	//dv.sums = make([]Fr, dv.pk.Count)
	for i := 0; i < int(dv.pk.Count); i++ {
		bls.FrClear(&dv.sums[i])
	}

	bls.G1Clear(&dv.delta)
	bls.FrClear(&dv.HWi)
}

type ProofVerifier struct {
	vk     *VerifyKey
	HWi    Fr
	tempFr Fr
}

func NewProofVerifier(vki pdpcommon.VerifyKey) pdpcommon.ProofVerifier {
	vk, ok := vki.(*VerifyKey)
	if !ok || vk == nil {
		return nil
	}
	var HWi Fr
	return &ProofVerifier{
		vk:  vk,
		HWi: HWi,
	}
}

func (pv *ProofVerifier) Version() int {
	return 2
}

func (pv *ProofVerifier) Reset() {
	bls.FrClear(&pv.HWi)
}

func (pv *ProofVerifier) Input(index []byte) error {
	h := blake3.Sum256([]byte(index))
	bls.FrFromBytes(&pv.tempFr, h[:])
	bls.FrAddMod(&pv.HWi, &pv.HWi, &pv.tempFr)
	return nil
}

func (pv *ProofVerifier) InputMulti(indices [][]byte) error {
	var tempFr Fr
	for _, index := range indices {
		h := blake3.Sum256([]byte(index))
		bls.FrFromBytes(&tempFr, h[:])
		bls.FrAddMod(&pv.HWi, &pv.HWi, &tempFr)
	}
	return nil
}

func (pv *ProofVerifier) Result(random int64, proof pdpcommon.Proof) bool {
	pf, ok := proof.(*Proof)
	if !ok {
		return false
	}
	var psi, kappa G1

	err := bls.G1Deserialize(&psi, pf.Psi)
	if err != nil {
		return false
	}

	if bls.G1IsZero(&psi) {
		return false
	}

	err = bls.G1Deserialize(&kappa, pf.Kappa)
	if err != nil {
		return false
	}

	if bls.G1IsZero(&kappa) {
		return false
	}

	var ProdHWi G1
	var G1temp1 G1
	var G2temp1 G2

	bls.G1Mul(&ProdHWi, &pv.vk.Phi, &pv.HWi)
	bls.G1Sub(&G1temp1, &kappa, &ProdHWi)

	bls.FrSetInt64(&pv.tempFr, random)
	bls.G2Mul(&G2temp1, &pv.vk.BlsPk, &pv.tempFr)
	bls.G2Sub(&G2temp1, &pv.vk.Zeta, &G2temp1)

	res := bls.PairingsVerify(
		&G1temp1, &GenG2, &psi, &G2temp1,
	)

	return res
}
