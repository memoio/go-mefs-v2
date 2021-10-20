package pdpv2

import (
	"encoding/base64"
	"math/rand"
	"strconv"
	"testing"
	"time"

	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12_381"
	"github.com/zeebo/blake3"
)

const chunkSize = DefaultSegSize
const FileSize = 512 * chunkSize
const SegNum = FileSize / DefaultSegSize

func TestVerifyProof(t *testing.T) {
	data := make([]byte, FileSize)
	rand.Seed(time.Now().UnixNano())
	fillRandom(data)

	// generate the key set for proof of data possession
	keySet, err := GenKeySetWithSeed(data[:32], SCount)
	if err != nil {
		panic(err)
	}

	pk := keySet.Pk
	vk := keySet.VerifyKey()
	segments := make([][]byte, SegNum)
	blocks := make([][]byte, SegNum)
	for i := 0; i < SegNum; i++ {
		segments[i] = data[DefaultSegSize*i : DefaultSegSize*(i+1)]
		blocks[i] = []byte(strconv.Itoa(i))
	}

	// ------------- the data owner --------------- //
	tags := make([][]byte, SegNum)
	for i, segment := range segments {
		// generate the data tag
		tag, err := keySet.GenTag([]byte(blocks[i]), segment, 0, true)
		if err != nil {
			panic("gentag Error")
		}

		// ok, _ := pk.VerifyTag([]byte(blocks[i]), segment, tag, DefaultType)
		// if !ok {
		// 	panic("VerifyTag failed")
		// }
		tags[i] = tag
	}

	pkBytes := pk.Serialize()
	pkDes := new(PublicKey)
	err = pkDes.Deserialize(pkBytes)
	if err != nil {
		t.Fatal(err)
	}

	if !bls.G2Equal(&pk.BlsPk, &pk.BlsPk) {
		t.Fatal("not equal")
	}

	// -------------- TPA --------------- //
	// generate the challenge for data possession validation
	chal := Challenge{
		r:       time.Now().Unix(),
		indices: blocks,
	}

	// generate the proof
	proof, err := pkDes.GenProof(&chal, segments, tags, DefaultType)
	if err != nil {
		panic(err)
	}

	t.Logf("%v", proof)

	vkBytes := vk.Serialize()
	vkDes := new(VerifyKey)
	err = vkDes.Deserialize(vkBytes)
	if err != nil {
		t.Fatal(err)
	}

	// -------------- TPA --------------- //
	// Verify the proof
	result, err := vkDes.VerifyProof(&chal, proof)
	if err != nil {
		t.Fatal(err)
	}
	if !result {
		t.Errorf("Verificaition failed!")
	}
}

func TestProofAggregator(t *testing.T) {
	data := make([]byte, FileSize)
	rand.Seed(time.Now().UnixNano())
	fillRandom(data)

	// generate the key set for proof of data possession
	keySet, err := GenKeySetWithSeed(data[:32], SCount)
	if err != nil {
		panic(err)
	}

	pk := keySet.PublicKey()
	vk := keySet.VerifyKey()
	segments := make([][]byte, SegNum)
	blocks := make([][]byte, SegNum)
	for i := 0; i < SegNum; i++ {
		segments[i] = data[DefaultSegSize*i : DefaultSegSize*(i+1)]
		blocks[i] = []byte(strconv.Itoa(i))
	}

	// ------------- the data owner --------------- //
	tags := make([][]byte, SegNum)
	for i, segment := range segments {
		// generate the data tag
		tag, err := keySet.GenTag([]byte(blocks[i]), segment, 0, true)
		if err != nil {
			panic("gentag Error")
		}

		ok, _ := pk.VerifyTag([]byte(blocks[i]), segment, tag, DefaultType)
		if !ok {
			panic("VerifyTag failed")
		}
		tags[i] = tag
	}

	// -------------- TPA --------------- //
	// generate the challenge for data possession validation
	chal := Challenge{
		r:       time.Now().Unix(),
		indices: blocks,
	}

	proofAggregator := NewProofAggregator(keySet.Pk, chal.r, DefaultType)
	if proofAggregator == nil {
		panic("proofAggregator is nil")
	}
	err = proofAggregator.Input(segments[0], tags[0])
	if err != nil {
		panic(err.Error())
	}
	err = proofAggregator.InputMulti(segments[1:], tags[1:])
	if err != nil {
		panic(err.Error())
	}

	proof, err := proofAggregator.Result()
	if err != nil {
		panic(err.Error())
	}

	// -------------- TPA --------------- //
	// Verify the proof
	result, err := vk.VerifyProof(&chal, proof)
	if err != nil {
		t.Fatal(err)
	}
	if !result {
		t.Errorf("Verificaition failed!")
	}
}

func TestDataVerifier(t *testing.T) {
	data := make([]byte, FileSize)
	rand.Seed(time.Now().UnixNano())
	fillRandom(data)

	// generate the key set for proof of data possession
	keySet, err := GenKeySetWithSeed(data[:32], SCount)
	if err != nil {
		panic(err)
	}

	pk := keySet.PublicKey()
	segments := make([][]byte, SegNum)
	blocks := make([][]byte, SegNum)
	for i := 0; i < SegNum; i++ {
		segments[i] = data[DefaultSegSize*i : DefaultSegSize*(i+1)]
		blocks[i] = []byte(strconv.Itoa(i))
	}

	// ------------- the data owner --------------- //
	tags := make([][]byte, SegNum)
	for i, segment := range segments {
		// generate the data tag
		tag, err := keySet.GenTag([]byte(blocks[i]), segment, 0, true)
		if err != nil {
			panic("gentag Error")
		}

		ok, _ := pk.VerifyTag([]byte(blocks[i]), segment, tag, DefaultType)
		if !ok {
			panic("VerifyTag failed")
		}
		tags[i] = tag
	}

	// -------------- TPA --------------- //
	// generate the challenge for data possession validation

	dataVerifier := NewDataVerifier(keySet.Pk, keySet.Sk)
	if dataVerifier == nil {
		panic("dataVerifier is nil")
	}
	err = dataVerifier.Input(blocks[0], segments[0], tags[0])
	if err != nil {
		panic(err.Error())
	}
	err = dataVerifier.InputMulti(blocks[1:], segments[1:], tags[1:])
	if err != nil {
		panic(err.Error())
	}

	result := dataVerifier.Result()
	if !result {
		t.Errorf("Verificaition failed!")
	}
}

func TestProofVerifier(t *testing.T) {
	data := make([]byte, FileSize)
	rand.Seed(time.Now().UnixNano())
	fillRandom(data)

	// generate the key set for proof of data possession
	keySet, err := GenKeySetWithSeed(data[:32], SCount)
	if err != nil {
		panic(err)
	}

	pk := keySet.Pk
	vk := keySet.VerifyKey()
	segments := make([][]byte, SegNum)
	blocks := make([][]byte, SegNum)
	for i := 0; i < SegNum; i++ {
		segments[i] = data[DefaultSegSize*i : DefaultSegSize*(i+1)]
		blocks[i] = []byte(strconv.Itoa(i))
	}

	// ------------- the data owner --------------- //
	tags := make([][]byte, SegNum)
	for i, segment := range segments {
		// generate the data tag
		tag, err := keySet.GenTag([]byte(blocks[i]), segment, 0, true)
		if err != nil {
			panic("gentag Error")
		}

		// ok, _ := pk.VerifyTag([]byte(blocks[i]), segment, tag, DefaultType)
		// if !ok {
		// 	panic("VerifyTag failed")
		// }
		tags[i] = tag
	}

	pkBytes := pk.Serialize()
	pkDes := new(PublicKey)
	err = pkDes.Deserialize(pkBytes)
	if err != nil {
		t.Fatal(err)
	}

	if !bls.G2Equal(&pk.BlsPk, &pk.BlsPk) {
		t.Fatal("not equal")
	}

	// -------------- TPA --------------- //
	// generate the challenge for data possession validation
	chal := Challenge{
		r:       time.Now().Unix(),
		indices: blocks,
	}

	// generate the proof
	proof, err := pkDes.GenProof(&chal, segments, tags, DefaultType)
	if err != nil {
		panic(err)
	}

	t.Logf("%v", proof)

	vkBytes := vk.Serialize()
	vkDes := new(VerifyKey)
	err = vkDes.Deserialize(vkBytes)
	if err != nil {
		t.Fatal(err)
	}

	// -------------- TPA --------------- //
	// Verify the proof

	pv := NewProofVerifier(vk)
	for _, v := range chal.indices {
		pv.Input(v)
	}

	result := pv.Result(chal.r, proof)
	if !result {
		t.Errorf("Verificaition failed!")
	}
}

func TestVerifyData(t *testing.T) {
	data := make([]byte, FileSize)
	rand.Seed(time.Now().UnixNano())
	fillRandom(data)

	// generate the key set for proof of data possession
	keySet, err := GenKeySetWithSeed(data[:32], SCount)
	if err != nil {
		panic(err)
	}

	pk := keySet.PublicKey()
	segments := make([][]byte, SegNum)
	blocks := make([][]byte, SegNum)
	for i := 0; i < SegNum; i++ {
		segments[i] = data[DefaultSegSize*i : DefaultSegSize*(i+1)]
		blocks[i] = []byte(strconv.Itoa(i))
	}

	// ------------- the data owner --------------- //
	tags := make([][]byte, SegNum)
	for i, segment := range segments {
		// generate the data tag
		tag, err := keySet.GenTag([]byte(blocks[i]), segment, 0, true)
		if err != nil {
			panic("gentag Error")
		}

		ok, _ := pk.VerifyTag([]byte(blocks[i]), segment, tag, DefaultType)
		if !ok {
			panic("VerifyTag failed")
		}
		tags[i] = tag
	}

	// -------------- TPA --------------- //
	// generate the challenge for data possession validation

	result, err := keySet.VerifyData(blocks, segments, tags)
	if err != nil {
		panic(err.Error())
	}
	if err != nil {
		t.Fatal(err)
	}
	if !result {
		t.Errorf("Verificaition failed!")
	}
}

func TestKeyDeserialize(t *testing.T) {
	data := make([]byte, FileSize)
	rand.Seed(time.Now().UnixNano())
	fillRandom(data)

	// generate the key set for proof of data possession
	keySet, err := GenKeySetWithSeed(data[:32], SCount)
	if err != nil {
		panic(err)
	}
	sk := keySet.Sk
	skBytes := sk.Serialize()
	skDes := new(SecretKey)
	err = skDes.Deserialize(skBytes)
	if err != nil {
		t.Fatal(err)
	}

	if !bls.FrEqual(&skDes.Alpha, &sk.Alpha) ||
		!bls.FrEqual(&skDes.BlsSk, &sk.BlsSk) ||
		!bls.FrEqual(&skDes.ElemAlpha[0], &sk.ElemAlpha[0]) {
		t.Fatal("not equal")
	}

	pk := keySet.Pk
	pkBytes := pk.Serialize()
	pkDes := new(PublicKey)
	err = pkDes.Deserialize(pkBytes)
	if err != nil {
		t.Fatal(err)
	}
	if !bls.G2Equal(&pkDes.BlsPk, &pk.BlsPk) || !bls.G2Equal(&pkDes.Zeta, &pk.Zeta) {
		t.Fatal("not equal")
	}

}

func TestFsID(t *testing.T) {
	data := make([]byte, FileSize)
	rand.Seed(time.Now().UnixNano())
	fillRandom(data)

	// generate the key set for proof of data possession
	keySet, err := GenKeySetWithSeed(data[:32], SCount)
	if err != nil {
		panic(err)
	}
	vk := keySet.VerifyKey().Serialize()
	fsIDBytes := blake3.Sum256(vk)

	fsID := base64.StdEncoding.EncodeToString(fsIDBytes[:20])

	t.Error(len(fsID), fsID)
}
