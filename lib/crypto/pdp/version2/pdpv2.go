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

		if len(index) != 32 {
			h := blake3.Sum256(index)
			bls.FrFromBytes(&HWi, h[:])
		} else {
			bls.FrFromBytes(&HWi, index)
		}

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

func (k *KeySet) VerifyTag(indice, segment, tag []byte) (bool, error) {
	if k.Pk == nil {
		return false, pdpcommon.ErrKeyIsNil
	}

	if len(segment) == 0 {
		return false, pdpcommon.ErrSegmentSize
	}

	tagNum := len(segment) / k.typ
	if len(k.Pk.ElemAlphas) < tagNum {
		return false, pdpcommon.ErrNumOutOfRange
	}

	atoms, err := splitSegmentToAtoms(segment, k.typ)
	if err != nil {
		return false, err
	}

	// muProd = Prod(u_j^sums_j)
	var muProd G1
	bls.G1Clear(&muProd)
	if k.Sk != nil {
		var power Fr
		bls.EvalPolyAt(&power, atoms, &(k.Sk.Alpha))
		bls.G1Mul(&muProd, &(GenG1), &power)
	} else {
		bls.G1MulVec(&muProd, k.Pk.ElemAlphas, atoms)
	}

	var delta G1
	err = bls.G1Deserialize(&delta, tag)
	if err != nil {
		return false, err
	}

	var ProdHWi, ProdHWimu G1
	var HWi Fr
	if len(indice) != 32 {
		h := blake3.Sum256(indice)
		bls.FrFromBytes(&HWi, h[:])
	} else {
		bls.FrFromBytes(&HWi, indice)
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
		return false, pdpcommon.ErrVerifyFailed
	}

	return true, nil
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
	if len(index) != 32 {
		h := blake3.Sum256(index)
		bls.FrFromBytes(&HWi, h[:])
	} else {
		bls.FrFromBytes(&HWi, index)
	}
	bls.G1Mul(&HWiG1, &GenG1, &HWi)

	err := bls.G1Deserialize(&delta, tag)
	if err != nil {
		return false, err
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
func (pk *PublicKey) GenProof(chal pdpcommon.Challenge, segment, tag []byte, typ int) (pdpcommon.Proof, error) {
	if pk == nil || typ <= 0 {
		return nil, pdpcommon.ErrKeyIsNil
	}
	// sums_j为待挑战的各segments位于同一位置(即j)上的atom的和
	if len(segment) == 0 {
		return nil, pdpcommon.ErrSegmentSize
	}

	tagNum := len(segment) / typ
	if len(pk.ElemAlphas) < tagNum {
		return nil, pdpcommon.ErrNumOutOfRange
	}

	atoms, err := splitSegmentToAtoms(segment, typ)
	if err != nil {
		return nil, err
	}

	var fr_r, pk_r Fr //P_k(r)
	rnd := chal.Random()
	bls.FrFromBytes(&fr_r, rnd[:])
	bls.EvalPolyAt(&pk_r, atoms, &fr_r)

	// poly(x) - poly(r) always divides (x - r) since the latter is a root of the former.
	// With that infomation we can jump into the division.
	quotient := make([]Fr, len(atoms)-1)
	if len(quotient) > 0 {
		// q_(n - 1) = p_n
		quotient[len(quotient)-1] = atoms[len(quotient)]
		for j := len(quotient) - 2; j >= 0; j-- {
			// q_j = p_(j + 1) + q_(j + 1) * r
			bls.FrMulMod(&quotient[j], &quotient[j+1], &fr_r)
			bls.FrAddMod(&quotient[j], &quotient[j], &atoms[j+1])
		}
	}
	var psi G1

	bls.G1MulVec(&psi, pk.ElemAlphas[:len(quotient)], quotient)

	// delta = Prod(tag_i)
	var delta G1
	err = bls.G1Deserialize(&delta, tag)
	if err != nil {
		return nil, err
	}

	var t, kappa G1
	bls.G1Mul(&t, &pk.Phi, &pk_r)
	bls.G1Sub(&kappa, &delta, &t)

	return &Proof{
		Psi:   psi,
		Kappa: kappa,
	}, nil
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

	var ProdHWi G1
	var G1temp1 G1
	var G2temp1 G2
	var tempFr, HWi Fr
	//var lhs1, lhs2, rhs1 GT

	bls.FrClear(&HWi)
	bls.FrFromBytes(&HWi, chal.PublicInput())

	bls.G1Mul(&ProdHWi, &vk.Phi, &HWi)
	bls.G1Sub(&G1temp1, &pf.Kappa, &ProdHWi)

	rnd := chal.Random()
	bls.FrFromBytes(&tempFr, rnd[:])
	bls.G2Mul(&G2temp1, &vk.BlsPk, &tempFr)
	bls.G2Sub(&G2temp1, &vk.Zeta, &G2temp1)

	res := bls.PairingsVerify(
		&G1temp1, &GenG2, &pf.Psi, &G2temp1,
	)
	return res, nil
}

type ProofAggregator struct {
	pk     *PublicKey
	r      [32]byte
	typ    int
	sums   []Fr
	delta  G1
	tempG1 G1
	tempFr Fr
	HWi    Fr
}

func NewProofAggregator(pki pdpcommon.PublicKey, seed [32]byte) pdpcommon.ProofAggregator {
	pk, ok := pki.(*PublicKey)
	if !ok {
		return nil
	}
	sums := make([]Fr, pk.Count)
	var delta G1
	bls.G1Clear(&delta)
	var tempG1 G1
	var tempFr Fr
	var HWi Fr
	return &ProofAggregator{pk, seed, DefaultType, sums, delta, tempG1, tempFr, HWi}
}

func (pa *ProofAggregator) Version() uint16 {
	return pdpcommon.PDPV2
}

func (pa *ProofAggregator) Add(index, segment, tag []byte) error {
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

	if len(index) != 32 {
		h := blake3.Sum256(index)
		bls.FrFromBytes(&pa.tempFr, h[:])
	} else {
		bls.FrFromBytes(&pa.tempFr, index)
	}

	bls.FrAddMod(&pa.HWi, &pa.HWi, &pa.tempFr)
	return nil
}

// if proof
func (pa *ProofAggregator) Result() (pdpcommon.Proof, error) {
	var pk_r Fr //P_k(r)
	var fr_r Fr
	bls.FrFromBytes(&fr_r, pa.r[:])
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

	// 返回证明的时候验证一下
	var ProdHWi G1
	var G1temp1 G1
	var G2temp1 G2

	bls.G1Mul(&ProdHWi, &pa.pk.Phi, &pa.HWi)
	bls.G1Sub(&G1temp1, &kappa, &ProdHWi)

	bls.FrFromBytes(&pa.tempFr, pa.r[:])
	bls.G2Mul(&G2temp1, &pa.pk.BlsPk, &pa.tempFr)
	bls.G2Sub(&G2temp1, &pa.pk.Zeta, &G2temp1)

	res := bls.PairingsVerify(
		&G1temp1, &GenG2, &psi, &G2temp1,
	)

	if !res {
		return &Proof{
			Psi:   psi,
			Kappa: kappa,
		}, pdpcommon.ErrVerifyFailed
	}

	return &Proof{
		Psi:   psi,
		Kappa: kappa,
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

func (dv *DataVerifier) Version() uint16 {
	return pdpcommon.PDPV2
}

func (dv *DataVerifier) Add(index, segment, tag []byte) error {
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
	if len(index) != 32 {
		h := blake3.Sum256(index)
		bls.FrFromBytes(&tempFr, h[:])
	} else {
		bls.FrFromBytes(&tempFr, index)
	}

	bls.FrAddMod(&dv.HWi, &dv.HWi, &tempFr)

	return nil
}

func (dv *DataVerifier) Result() (bool, error) {
	var muProd G1
	bls.G1Clear(&muProd)
	if dv.sk != nil {
		var power Fr
		bls.EvalPolyAt(&power, dv.sums, &(dv.sk.Alpha))
		bls.G1Mul(&muProd, &(GenG1), &power)
	} else {
		bls.G1MulVec(&muProd, dv.pk.ElemAlphas, dv.sums)
	}

	var ProdHWimu G1
	// var index string
	// var offset int
	// 第一步：验证tag和mu是对应的
	// lhs = e(delta, g)

	bls.G1Mul(&ProdHWimu, &GenG1, &dv.HWi)
	bls.G1Add(&ProdHWimu, &ProdHWimu, &muProd)

	return bls.PairingsVerify(&dv.delta, &GenG2, &ProdHWimu, &dv.pk.BlsPk), nil
}

func (dv *DataVerifier) Reset() {
	//dv.sums = make([]Fr, dv.pk.Count)
	for i := 0; i < int(dv.pk.Count); i++ {
		bls.FrClear(&dv.sums[i])
	}

	bls.G1Clear(&dv.delta)
	bls.FrClear(&dv.HWi)
}
