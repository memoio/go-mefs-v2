package code

import (
	"crypto/md5"
	"crypto/sha256"
	"math/rand"
	"testing"
	"time"

	"github.com/memoio/go-mefs-v2/lib/crypto/aes"
	"github.com/memoio/go-mefs-v2/lib/crypto/pdp"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/zeebo/blake3"
)

// 全局配置
var skbyte = []byte{179, 233, 48, 97, 94, 148, 140, 7, 78, 102, 169, 48, 136, 124, 152, 101, 76, 69, 210, 14, 38, 15, 176, 227, 73, 41, 135, 17, 170, 138, 242, 69}
var buckid = 1

// 因只考虑生成3+2个stripe，故测试Rs时，文件长度不超过3M；测试Mul时，文件长度不超过1M
var Rslen = 2 * 1024 * 1024
var Mullen = 1 * 1024 * 1024

var userID = "QmRL8b4C2wUJLTceEYsDXeBJBxG1ki8zRoAUJEEvPG952G"

var FileSize = 56 * DefaultSegSize

func BenchmarkEncode(b *testing.B) {
	keyset, err := pdp.GenerateKey(pdpcommon.PDPV2)
	if err != nil {
		b.Fatal(err)
	}
	dataCount := 28
	parityCount := 52

	opt, err := NewDataCoderWithDefault(keyset, RsPolicy, dataCount, parityCount, userID, userID)
	if err != nil {
		b.Fatal(err)
	}

	data := make([]byte, dataCount*DefaultSegSize)
	fillRandom(data)
	b.SetBytes(int64(dataCount * DefaultSegSize))
	b.ResetTimer()

	fsID := blake3.Sum256([]byte("QmRL8b4C2wUJLTceEYsDXeBJBxG1ki8zRoAUJEEvPG952G"))
	segID, err := segment.NewSegmentID(fsID[:20], 0, 0, 0)
	if err != nil {
		b.Fatal(err)
	}

	for i := 0; i < b.N; i++ {
		//一次编码一个stripe
		for j := 0; j < len(data); j += DefaultSegSize * dataCount {

			var d []byte
			if len(data)-j >= DefaultSegSize*dataCount {
				d = data[j : j+DefaultSegSize*dataCount]
			} else {
				d = data[j:]
			}
			// 加密、Encode
			// 构建加密秘钥icy
			tmpkey := append(skbyte, byte(buckid))
			skey := sha256.Sum256(tmpkey)
			Encdata, _ := aes.AesEncrypt(d, skey[:])
			// Encdata := d

			// 多副本含前缀
			_, err = opt.Encode(segID, Encdata)
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkDecodeFast(b *testing.B) {
	keyset, err := pdp.GenerateKey(pdpcommon.PDPV2)
	if err != nil {
		b.Fatal(err)
	}
	dataCount := 28
	parityCount := 52

	opt, err := NewDataCoderWithDefault(keyset, RsPolicy, dataCount, parityCount, userID, userID)
	if err != nil {
		b.Fatal(err)
	}

	fsID := blake3.Sum256([]byte("QmRL8b4C2wUJLTceEYsDXeBJBxG1ki8zRoAUJEEvPG952G"))
	segID, err := segment.NewSegmentID(fsID[:20], 0, 0, 0)
	if err != nil {
		b.Fatal(err)
	}

	data := make([]byte, dataCount*DefaultSegSize)
	fillRandom(data)
	b.SetBytes(int64(dataCount * DefaultSegSize))

	// 加密、Encode
	// 构建加密秘钥icy
	tmpkey := append(skbyte, byte(buckid))
	skey := sha256.Sum256(tmpkey)
	Encdata, _ := aes.AesEncrypt(data, skey[:])

	datas, err := opt.Encode(segID, Encdata)
	if err != nil {
		b.Fatal(err)
	}

	var gotData []byte

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < parityCount; j++ {
			datas[(i+j)%(dataCount+parityCount)] = nil
		}

		gotData, err = opt.Decode(nil, datas)
		if err != nil {
			b.Fatal(i, err)
		}

		if i%(dataCount+parityCount) == dataCount {
			opt.Recover(nil, datas)
		}
	}

	gotData, _ = aes.AesDecrypt(gotData, skey[:])

	if md5.Sum(gotData) != md5.Sum(data) {
		b.Fatal("Not equal")
	}
}

func BenchmarkDecode(b *testing.B) {
	keyset, err := pdp.GenerateKey(pdpcommon.PDPV2)
	if err != nil {
		b.Fatal(err)
	}
	dataCount := 28
	parityCount := 52

	opt, err := NewDataCoderWithDefault(keyset, RsPolicy, dataCount, parityCount, userID, userID)
	if err != nil {
		b.Fatal(err)
	}

	fsID := blake3.Sum256([]byte("QmRL8b4C2wUJLTceEYsDXeBJBxG1ki8zRoAUJEEvPG952G"))
	segID, err := segment.NewSegmentID(fsID[:20], 0, 0, 0)
	if err != nil {
		b.Fatal(err)
	}

	data := make([]byte, dataCount*DefaultSegSize)
	fillRandom(data)
	b.SetBytes(int64(dataCount * DefaultSegSize))

	// 加密、Encode
	// 构建加密秘钥icy
	tmpkey := append(skbyte, byte(buckid))
	skey := sha256.Sum256(tmpkey)
	Encdata, _ := aes.AesEncrypt(data, skey[:])

	datas, err := opt.Encode(segID, Encdata)
	if err != nil {
		b.Fatal(err)
	}

	var gotData []byte
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < parityCount; j++ {
			datas[(i+j)%(dataCount+parityCount)] = nil
		}

		gotData, err = opt.Decode(segID, datas)
		if err != nil {
			b.Fatal(err)
		}

		if i%(dataCount+parityCount) == dataCount {
			opt.Recover(segID, datas)
		}
	}

	gotData, _ = aes.AesDecrypt(gotData, skey[:])

	if md5.Sum(gotData) != md5.Sum(data) {
		b.Fatal("Not equal")
	}
}

func CodeAndRepair(policy, dc, pc, dlen int, t *testing.T) {
	keyset, err := pdp.GenerateKey(pdpcommon.PDPV2)
	if err != nil {
		t.Fatal(err)
	}

	opt, err := NewDataCoderWithDefault(keyset, policy, dc, pc, userID, userID)
	if err != nil {
		t.Fatal(err)
	}

	fsID := blake3.Sum256([]byte("QmRL8b4C2wUJLTceEYsDXeBJBxG1ki8zRoAUJEEvPG952G"))
	segID, err := segment.NewSegmentID(fsID[:20], 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	data := make([]byte, dlen)
	fillRandom(data)

	oldMd5 := md5.Sum(data)
	t.Logf("data md5 is: %d, len is: %d\n", oldMd5, len(data))

	tmpkey := append(skbyte, byte(buckid))
	skey := sha256.Sum256(tmpkey)
	// 加密、Encode

	if len(data)%aes.BlockSize != 0 {
		data = aes.PKCS5Padding(data)
	}

	data, _ = aes.AesEncrypt(data, skey[:])
	// 多副本含前缀
	datas, err := opt.Encode(segID, data)
	if err != nil {
		t.Fatal(err)
	}

	_, good, err := opt.VerifyStripe(segID, datas)
	if err != nil {
		t.Fatal(err)
	}

	if good < len(datas) {
		t.Fatal("wrong encode")
	}

	gotdata, err := opt.Decode(segID, datas)
	if err != nil {
		t.Fatal(err)
	}

	gotdata, _ = aes.AesDecrypt(gotdata, skey[:])

	gotMd5 := md5.Sum(gotdata[:dlen])
	t.Log("data md5 is, len is: ", gotMd5, len(gotdata))
	if gotMd5 != oldMd5 {
		t.Fatal("Md5 is not right after decode")
	}

	t.Log("decode success")

	datas[2] = nil

	t.Log("data:", datas[1][:DefaultPrefixLen])

	datas, err = Repair(keyset, segID, datas)
	if err != nil {
		t.Fatal("Repair ", err)
	}

	_, good, err = opt.VerifyStripe(segID, datas)
	if err != nil {
		t.Fatal(err)
	}

	if good < len(datas) {
		t.Fatal("wrong encode")
	}

	gotdata, err = opt.Decode(segID, datas)
	if err != nil {
		t.Fatal("Decode ", err)
	}

	gotdata, _ = aes.AesDecrypt(gotdata, skey[:])

	gotMd5 = md5.Sum(gotdata[:dlen])
	t.Log("data md5 is: ", gotMd5)
	if gotMd5 != oldMd5 {
		t.Fatal("Md5 is not right after decode 2")
	}

	t.Log("repair one success")

	datas[0] = nil
	datas[dc] = nil

	datas, err = Repair(keyset, segID, datas)
	if err != nil {
		t.Fatal("Repair2 ", err)
	}

	gotdata, err = opt.Decode(segID, datas)
	if err != nil {
		t.Fatal("Decode2 ", err)
	}

	gotdata, _ = aes.AesDecrypt(gotdata, skey[:])

	gotMd5 = md5.Sum(gotdata[:dlen])
	t.Log("data md5 is: ", gotMd5)
	if gotMd5 != oldMd5 {
		t.Fatal("Md5 is not right after repair")
	}
	t.Log("repair two success")

	if policy == RsPolicy {
		datas[0] = nil
		datas[dc] = nil

		gotdata, err = opt.Decode(segID, datas)
		if err != nil {
			t.Fatal("Decode3 ", err)
		}

		gotdata, _ = aes.AesDecrypt(gotdata, skey[:])

		gotMd5 = md5.Sum(gotdata[:dlen])
		t.Log("data md5 is: ", gotMd5)
		if gotMd5 != oldMd5 {
			t.Fatal("Md5 is not right after repair")
		}
		t.Log("rs decode success")
	}
}

func TestCode(t *testing.T) {
	t.Log("test size: ", 29*DefaultSegSize)
	CodeAndRepair(RsPolicy, 29, 51, 29*DefaultSegSize, t)
	CodeAndRepair(MulPolicy, 3, 2, DefaultSegSize, t)
}

func TestStripe(t *testing.T) {
	keyset, err := pdp.GenerateKey(pdpcommon.PDPV2)
	if err != nil {
		t.Fatal(err)
	}
	dataCount := 28
	parityCount := 52
	policy := RsPolicy

	dlen := dataCount * DefaultSegSize

	opt, err := NewDataCoderWithDefault(keyset, policy, dataCount, parityCount, userID, userID)
	if err != nil {
		t.Fatal(err)
	}

	fsID := blake3.Sum256([]byte("QmRL8b4C2wUJLTceEYsDXeBJBxG1ki8zRoAUJEEvPG952G"))
	segID, err := segment.NewSegmentID(fsID[:20], 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	data := make([]byte, dlen)
	fillRandom(data)

	oldMd5 := md5.Sum(data)

	tmpkey := append(skbyte, byte(buckid))
	skey := sha256.Sum256(tmpkey)

	if len(data)%aes.BlockSize != 0 {
		data = aes.PKCS5Padding(data)
	}
	data, _ = aes.AesEncrypt(data, skey[:])

	datas, err := opt.Encode(segID, data)
	if err != nil {
		t.Fatal(err)
	}

	_, good, err := opt.VerifyStripe(segID, datas)
	if err != nil {
		t.Fatal(err)
	}

	if good < len(datas) {
		t.Fatal("wrong encode")
	}

	gotdata, err := opt.Decode(segID, datas)
	if err != nil {
		t.Fatal(err)
	}

	gotdata, _ = aes.AesDecrypt(gotdata, skey[:])

	gotMd5 := md5.Sum(gotdata[:dlen])
	t.Log("data md5 is: ", gotMd5)
	if gotMd5 != oldMd5 {
		t.Fatal("Md5 is not right after decode")
	}

	t.Log("decode success")

	datas[1] = nil
	datas[28] = nil

	gotdata, err = opt.Decode(nil, datas)
	if err != nil {
		t.Fatal("Decode2 ", err)
	}

	gotdata, _ = aes.AesDecrypt(gotdata, skey[:])

	gotMd5 = md5.Sum(gotdata[:dlen])
	t.Log("data md5 is: ", gotMd5)
	if gotMd5 != oldMd5 {
		t.Fatal("Md5 is not right after repair")
	}
	t.Log("recover first success")

	// =====================================
	// data is wrong
	chunkLen := len(datas[0])
	wdata := make([]byte, chunkLen-24)
	fillRandom(wdata)

	wdata1 := make([]byte, chunkLen+100)
	fillRandom(wdata1)
	datas[0] = wdata
	datas[1] = nil
	datas[28] = nil
	datas[29] = wdata1

	_, good, err = opt.VerifyStripe(segID, datas)
	if err != nil {
		t.Fatal(err)
	}

	if good != len(datas)-4 {
		t.Fatal("wrong verify:", good, len(datas))
	}

	err = opt.Recover(segID, datas)
	if err != nil {
		t.Fatal(err)
	}

	_, good, err = opt.VerifyStripe(segID, datas)
	if err != nil {
		t.Fatal(err)
	}

	if good != len(datas) {
		t.Fatal("wrong verify after recover")
	}

	gotdata, err = opt.Decode(segID, datas)
	if err != nil {
		t.Fatal("Decode2 ", err)
	}

	gotdata, _ = aes.AesDecrypt(gotdata, skey[:])

	gotMd5 = md5.Sum(gotdata[:dlen])
	t.Log("data md5 is: ", gotMd5)
	if gotMd5 != oldMd5 {
		t.Fatal("Md5 is not right after repair")
	}
	t.Log("recover two success")

	// =====================================
	// data is wrong

	// tag is wrong and
	datas[1] = datas[1][:24]
	datas[1] = append(datas[1], wdata[:DefaultSegSize]...)
	datas[1] = append(datas[1], datas[0][24+DefaultSegSize:]...)
	datas[0] = nil
	datas[29] = nil
	datas[28] = wdata1

	_, err = opt.Decode(nil, datas)
	if err == nil {
		t.Fatal("Decode3 should not success")
	}

	t.Log("recover three fail: ", err)

	// =====================================
	// data is wrong

	_, good, err = opt.VerifyStripe(segID, datas)
	if err != nil {
		t.Fatal(err)
	}

	if good != len(datas)-4 {
		t.Fatal("wrong verify after recover: ", good, len(datas))
	}

	gotdata, err = opt.Decode(segID, datas)
	if err != nil {
		t.Fatal("Decode4 ", err)
	}

	gotdata, _ = aes.AesDecrypt(gotdata, skey[:])

	gotMd5 = md5.Sum(gotdata[:dlen])
	t.Log("data md5 is: ", gotMd5)
	if gotMd5 != oldMd5 {
		t.Fatal("Md5 is not right after repair")
	}
	t.Log("recover four success")

	if policy == RsPolicy {
		datas[0] = nil
		datas[4] = nil

		gotdata, err = opt.Decode(segID, datas)
		if err != nil {
			t.Fatal("Decode3 ", err)
		}

		gotdata, _ = aes.AesDecrypt(gotdata, skey[:])

		gotMd5 = md5.Sum(gotdata[:dlen])
		t.Log("data md5 is: ", gotMd5)
		if gotMd5 != oldMd5 {
			t.Fatal("Md5 is not right after repair")
		}
		t.Log("rs decode success")
	}
}

func fillRandom(p []byte) {
	rand.Seed(time.Now().UnixNano())
	for i := 0; i < len(p); i += 7 {
		val := rand.Int63()
		for j := 0; i+j < len(p) && j < 7; j++ {
			p[i+j] = byte(val)
			val >>= 8
		}
	}
}
