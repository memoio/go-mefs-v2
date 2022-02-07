package pdpv2

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"math/rand"
	"strconv"
	"testing"
	"time"

	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12_381"
	"github.com/zeebo/blake3"
	"lukechampine.com/frand"
)

// 测试tag的形成
func BenchmarkGenTag(b *testing.B) {
	// generate the key set for proof of data possession
	keySet, err := GenKeySet()
	if err != nil {
		panic(err)
	}

	b.Logf("FileSize: %.1f MB, Piece size: %d KB, Type size: %d Byte", float64(FileSize)/(1024*1024), DefaultSegSize/1024, DefaultType)

	// sample data
	// 10MB
	data := make([]byte, FileSize)
	rand.Seed(time.Now().UnixNano())
	fillRandom(data)

	segmentCount := (len(data) - 1) / DefaultSegSize
	segments := make([][]byte, segmentCount)
	for i := 0; i < segmentCount; i++ {
		segments[i] = data[DefaultSegSize*i : DefaultSegSize*(i+1)]
	}

	// ------------- the data owner --------------- //
	//tagTable := make(map[string][]byte)
	// add index/data pair
	// index := fmt.Sprintf("%s", sampleIdPrefix)
	b.SetBytes(int64(FileSize))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j, segment := range segments {
			// generate the data tag
			_, err = keySet.GenTag([]byte("123456"+strconv.Itoa(j)), segment, 0, true)
			if err != nil {
				log.Fatal(err)
			}
		}
	}
}

func BenchmarkGenOneTag(b *testing.B) {
	keySet, err := GenKeySet()
	if err != nil {
		panic(err)
	}
	// sample data
	data := make([]byte, DefaultSegSize)
	rand.Seed(time.Now().UnixNano())
	fillRandom(data)

	// ------------- the data owner --------------- //
	//tagTable := make(map[string][]byte)
	// add index/data pair
	// index := fmt.Sprintf("%s", sampleIdPrefix)
	b.SetBytes(int64(DefaultSegSize))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// generate the data tag
		_, err := keySet.GenTag([]byte("123456"+strconv.Itoa(i)), data, 0, true)
		if err != nil {
			b.Error(err)
		}
	}
}

func benchmarkGenOneTag(keySet *KeySet, pieceSize int) func(b *testing.B) {
	return func(b *testing.B) {
		// sample data
		data := make([]byte, pieceSize)
		rand.Seed(time.Now().UnixNano())
		fillRandom(data)

		// ------------- the data owner --------------- //
		//tagTable := make(map[string][]byte)
		// add index/data pair
		// index := fmt.Sprintf("%s", sampleIdPrefix)
		b.SetBytes(int64(pieceSize))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// generate the data tag
			_, err := keySet.GenTag([]byte("123456"+strconv.Itoa(i)), data, 0, true)
			if err != nil {
				b.Error(err)
			}
		}
	}
}

func BenchmarkMultiGenTag(b *testing.B) {
	keySet, err := GenKeySet()
	if err != nil {
		panic(err)
	}
	pieceSzie := 3968
	for i := 0; i < 7; i++ {
		b.Run("PieceSize:"+strconv.FormatFloat(float64(pieceSzie)/1024, 'f', -1, 64)+"Kb", benchmarkGenOneTag(keySet, pieceSzie))
		pieceSzie = pieceSzie * 2
	}
}

func GenTestChallenge(r uint64, prefix string, bound, count uint64) []string {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, r)
	seed := blake3.Sum256(buf)
	rng := frand.NewCustom(seed[:], 32, 20)

	res := make([]string, count)

	for i := 0; i < int(count); i++ {
		cid := rng.Uint64n(bound)
		cidstr := strconv.FormatUint(cid, 10)
		res[i] = prefix + cidstr
	}
	return nil
}

func TestRandom(t *testing.T) {
	seed := make([]byte, 32)
	for i := 0; i < 32; i++ {
		seed[i] = 2
	}
	rng := frand.NewCustom(seed[:], 32, 20)
	for i := 0; i < 100; i++ {
		cid := rng.Uint64n(math.MaxUint64)
		fmt.Println(cid)
	}
}

func BenchmarkGenChallenge(b *testing.B) {
	r := 18304
	prefix := "QmRL8b4C2wUJLTceEYsDXeBJBxG1ki8z_0"
	bound := 100000000
	count := 92099
	// -------------- TPA --------------- //
	// generate the challenge for data possession validation
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GenTestChallenge(uint64(r), prefix, uint64(bound), uint64(count))
	}
}

func BenchmarkGenProof(b *testing.B) {
	// generate the key set for proof of data possession
	keySet, err := GenKeySet()
	if err != nil {
		panic(err)
	}

	//vk := keySet.VerifyKey()
	data := make([]byte, FileSize)
	rand.Seed(time.Now().UnixNano())
	fillRandom(data)

	segments := make([][]byte, SegNum)
	chal := Challenge{
		r:        blake3.Sum256([]byte("test")),
		pubInput: bls.ZERO,
	}

	for i := 0; i < SegNum; i++ {
		segments[i] = data[DefaultSegSize*i : DefaultSegSize*(i+1)]
		chal.Add([]byte(strconv.Itoa(i) + "_" + "0"))
	}

	// ------------- the data owner --------------- //
	tags := make([][]byte, SegNum)
	for j, segment := range segments {
		// generate the data tag
		tags[j], err = keySet.GenTag([]byte(strconv.Itoa(j)+"_"+"0"), segment, 0, true)
		if err != nil {
			panic("Error" + err.Error())
		}
	}

	// -------------- TPA --------------- //
	// generate the challenge for data possession validation

	b.ResetTimer()
	b.SetBytes(int64(FileSize))
	for i := 0; i < b.N; i++ {
		// generate the proof
		_, err := keySet.Pk.GenProof(&chal, segments[0], tags[0], DefaultType)
		if err != nil {
			panic("Error" + err.Error())
		}
		// result, err := vk.VerifyProof(&chal, proof)
		// if err != nil {
		// 	panic("Error")
		// }
		// if !result {
		// 	b.Errorf("Verificaition failed!")
		// }
	}
}

func BenchmarkVerifyProof(b *testing.B) {
	// sample data
	data := make([]byte, FileSize)
	rand.Seed(time.Now().UnixNano())
	fillRandom(data)

	// generate the key set for proof of data possession
	keySet, err := GenKeySetWithSeed(data[:32], SCount)
	if err != nil {
		panic(err)
	}

	vk := keySet.VerifyKey()
	segments := make([][]byte, SegNum)
	chal := Challenge{
		r:        blake3.Sum256([]byte("test")),
		pubInput: bls.ZERO,
	}
	for i := 0; i < SegNum; i++ {
		segments[i] = data[DefaultSegSize*i : DefaultSegSize*(i+1)]
		chal.Add([]byte(strconv.Itoa(i)))
	}

	// ------------- the data owner --------------- //
	tags := make([][]byte, SegNum)
	for i, segment := range segments {
		// generate the data tag
		tags[i], err = keySet.GenTag([]byte(strconv.Itoa(i)), segment, 0, true)
		if err != nil {
			panic(err)
		}

		// ok, err := keySet.Pk.VerifyTag([]byte(blocks[i]), segment, tags[i], DefaultType)
		// if err != nil {
		// 	panic(err)
		// }
		// if !ok {
		// 	panic("VerifyTag false")
		// }
	}

	// -------------- TPA --------------- //
	// generate the challenge for data possession validation

	// generate the proof
	proof, err := keySet.Pk.GenProof(&chal, segments[0], tags[0], DefaultType)
	if err != nil {
		panic("Error")
	}

	// -------------- TPA --------------- //
	// Verify the proof
	b.ResetTimer()
	b.SetBytes(int64(FileSize))
	for i := 0; i < b.N; i++ {
		result, err := vk.VerifyProof(&chal, proof)
		if err != nil {
			panic("Error")
		}
		if !result {
			b.Errorf("Verificaition failed!")
		}
	}
}

func BenchmarkDataVerifier(b *testing.B) {
	data := make([]byte, FileSize)
	rand.Seed(time.Now().UnixNano())
	fillRandom(data)

	// generate the key set for proof of data possession
	keySet, err := GenKeySetWithSeed(data[:32], SCount)
	if err != nil {
		panic(err)
	}

	// pk := keySet.PublicKey()
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

		// res, _ := pk.VerifyTag([]byte(blocks[i]), segment, tag, DefaultType)
		// if !res {
		// 	panic("VerifyTag failed")
		// }
		tags[i] = tag
	}

	// -------------- TPA --------------- //
	// generate the challenge for data possession validation

	dataVerifier := NewDataVerifier(keySet.Pk, nil)
	if dataVerifier == nil {
		panic("dataVerifier is nil")
	}
	b.ResetTimer()
	b.SetBytes(int64(FileSize))
	for j := 0; j < b.N; j++ {
		for i := 0; i < len(segments); i++ {
			err = dataVerifier.Add(blocks[i], segments[i], tags[i])
			if err != nil {
				panic(err.Error())
			}
		}

		result, err := dataVerifier.Result()
		if !result || err != nil {
			b.Error("Verificaition failed!", err)
		}
	}
}

func BenchmarkProofAggregator(b *testing.B) {
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

	for i := 0; i < SegNum; i++ {
		segments[i] = data[DefaultSegSize*i : DefaultSegSize*(i+1)]
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

	b.SetBytes(int64(FileSize))
	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		proofAggregator := NewProofAggregator(keySet.Pk, chal.r)
		if proofAggregator == nil {
			panic("proofAggregator is nil")
		}
		for i := 0; i < len(segments); i++ {
			err = proofAggregator.Add([]byte(strconv.Itoa(i)), segments[i], tags[i])
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
			b.Fatal(err)
		}

		if !result {
			b.Errorf("Verificaition failed!")
		}
	}
}

func BenchmarkValidatePK(b *testing.B) {
	data := make([]byte, 32)
	rand.Seed(time.Now().UnixNano())
	fillRandom(data)

	// generate the key set for proof of data possession
	keySet, err := GenKeySetWithSeed(data[:32], SCount)
	if err != nil {
		panic(err)
	}
	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		ok := keySet.Pk.ValidateFast(keySet.VerifyKey().(*VerifyKey))
		if !ok {
			panic("error")
		}
	}
}

func BenchmarkPolyDiv(b *testing.B) {
	sums := make([]Fr, 8192)
	var r Fr
	bls.FrRandom(&r)
	for i := 0; i < len(sums); i++ {
		bls.FrRandom(&sums[i])
	}

	for i := 0; i < b.N; i++ {
		_ = Polydiv(sums, r)
	}
}
