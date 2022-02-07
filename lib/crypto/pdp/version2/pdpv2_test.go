package pdpv2

import (
	"bytes"
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

func fillRandom(p []byte) {
	for i := 0; i < len(p); i += 7 {
		val := rand.Int63()
		for j := 0; i+j < len(p) && j < 7; j++ {
			p[i+j] = byte(val)
			val >>= 8
		}
	}
}

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
	chal := Challenge{
		r:        blake3.Sum256([]byte("test")),
		pubInput: bls.ZERO,
	}

	chal.Add([]byte(strconv.Itoa(0)))

	for i := 0; i < SegNum; i++ {
		segments[i] = data[DefaultSegSize*i : DefaultSegSize*(i+1)]
	}

	// ------------- the data owner --------------- //
	tags := make([][]byte, SegNum)
	for i, segment := range segments {
		// generate the data tag
		tag, err := keySet.GenTag([]byte(strconv.Itoa(i)), segment, 0, true)
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

	// generate the proof
	proof, err := pkDes.GenProof(&chal, segments[0], tags[0], DefaultType)
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

	// pk := keySet.PublicKey()
	vk := keySet.VerifyKey()
	segments := make([][]byte, SegNum)
	chal := Challenge{
		r:        blake3.Sum256([]byte("test")),
		pubInput: bls.ZERO,
	}

	blocks := make([][]byte, SegNum)
	for i := 0; i < SegNum; i++ {
		segments[i] = data[DefaultSegSize*i : DefaultSegSize*(i+1)]
		blocks[i] = []byte(strconv.Itoa(i))
		chal.Add([]byte(strconv.Itoa(i)))
	}

	// ------------- the data owner --------------- //
	tags := make([][]byte, SegNum)
	for i, segment := range segments {
		// generate the data tag
		tag, err := keySet.GenTag([]byte(strconv.Itoa(i)), segment, 0, true)
		if err != nil {
			panic("gentag Error")
		}

		// ok, _ := pk.VerifyTag([]byte(blocks[i]), segment, tag, DefaultType)
		// if !ok {
		// 	panic("VerifyTag failed")
		// }
		tags[i] = tag
	}

	// -------------- TPA --------------- //
	// generate the challenge for data possession validation

	proofAggregator := NewProofAggregator(keySet.Pk, chal.r)
	if proofAggregator == nil {
		panic("proofAggregator is nil")
	}

	for i := 0; i < SegNum; i++ {
		err = proofAggregator.Add(blocks[i], segments[i], tags[i])
		if err != nil {
			panic(err.Error())
		}
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

	for i := 0; i < SegNum; i++ {
		err = dataVerifier.Add(blocks[i], segments[i], tags[i])
		if err != nil {
			panic(err.Error())
		}
	}

	result, err := dataVerifier.Result()
	if !result || err != nil {
		t.Error("Verificaition failed!", err)
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

		result, err := keySet.VerifyTag(blocks[i], segments[i], tags[i])
		if err != nil {
			panic(err)
		}
		if !result {
			t.Errorf("Verificaition failed!")
		}
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

	pkDesBytes := pkDes.Serialize()
	if !bytes.Equal(pkDesBytes, pkBytes) {
		t.Fatal("123")
	}
	if !bls.G2Equal(&pkDes.BlsPk, &pk.BlsPk) || !bls.G2Equal(&pkDes.Zeta, &pk.Zeta) {
		t.Fatal("not equal")
	}

}

func TestCreate(t *testing.T) {
	data := make([]byte, FileSize)
	rand.Seed(time.Now().UnixNano())
	fillRandom(data)

	tmp := bls.ZERO

	h := blake3.Sum256(data)
	bls.FrFromBytes(&tmp, h[:])

	t.Log(tmp.String())

	tmp2 := bls.ZERO

	h = blake3.Sum256(h[:])
	bls.FrFromBytes(&tmp2, h[:])

	t.Fatal(tmp.String(), tmp2.String())
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
