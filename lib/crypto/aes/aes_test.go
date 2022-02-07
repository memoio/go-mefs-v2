package aes

import (
	"bytes"
	"fmt"
	"math/rand"
	"testing"

	blake2bsimd "github.com/minio/blake2b-simd"
	"github.com/zeebo/blake3"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/blake2s"
)

func BenchmarkBlake2s256(b *testing.B) {
	var d1 [32]byte
	fmt.Println((d1))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d1 = blake2s.Sum256([]byte("1234567"))
	}
}

func BenchmarkBlake2b512(b *testing.B) {
	var d1 [64]byte
	fmt.Println((d1))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d1 = blake2b.Sum512([]byte("1234567sjjskdjkdlsllslagfgadddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddslkvjjk"))
	}
}
func BenchmarkBlake2b256(b *testing.B) {
	var d1 [32]byte
	fmt.Println((d1))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d1 = blake2b.Sum256([]byte("1234567sjjskdjkdlsllslagfgadddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddslkvjjk"))
	}
}

func BenchmarkBlake2bsimd512(b *testing.B) {
	var d1 [64]byte
	fmt.Println((d1))
	h := blake2bsimd.New512()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		h.Reset()
		h.Write([]byte("1234567sjjskdjkdlsllslagfgadddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddslkvjjk"))
		h.Sum(nil)
	}
}

func BenchmarkBlake3_512(b *testing.B) {
	var d1 [64]byte
	fmt.Println((d1))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d1 = blake3.Sum512([]byte("1234567sjjskdjkdlsllslagfgadddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddslkvjjk"))
	}
}
func BenchmarkBlake3_256(b *testing.B) {
	var d1 [32]byte
	fmt.Println((d1))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d1 = blake3.Sum256([]byte("1234567sjjskdjkdlsllslagfgadddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddslkvjjk"))
	}
}

func TestNewAes(t *testing.T) {
	key := make([]byte, KeySize)
	data := make([]byte, BlockSize*5)
	rand.Seed(0)
	fillRandom(key)
	fillRandom(data)
	fmt.Println(data)
	fmt.Println(key)

	bme, err := ContructAesEnc(key, 0, 0)
	if err != nil {
		t.Fatal("ContructAes error:", err)
	}

	bmd, err := ContructAesDec(key, 0, 0)
	if err != nil {
		t.Fatal("ContructAes error:", err)
	}

	dhash := blake2b.Sum256(data)

	crypted := make([]byte, len(data))
	bme.CryptBlocks(crypted[:2*BlockSize], data[:2*BlockSize])
	bme.CryptBlocks(crypted[2*BlockSize:], data[2*BlockSize:])

	//bme.CryptBlocks(crypted, data)
	data = make([]byte, len(data))
	//bmd.CryptBlocks(data, crypted)
	bmd.CryptBlocks(data[:3*BlockSize], crypted[:3*BlockSize])
	bmd.CryptBlocks(data[3*BlockSize:], crypted[3*BlockSize:])

	dhash2 := blake2b.Sum256(data)
	if !bytes.Equal(dhash[:], dhash2[:]) {
		fmt.Println(data)
		t.Fatal("decrypto error")
	}
}

func TestAes(t *testing.T) {
	key := make([]byte, KeySize)
	data := make([]byte, BlockSize*2)
	rand.Seed(0)
	fillRandom(key)
	fillRandom(data)

	ae, err := ContructAesEnc(key, 0, 0)
	if err != nil {
		t.Fatal("AesEncrypt error")
	}

	crypted := make([]byte, len(data))
	ae.CryptBlocks(crypted, data)

	ad, err := ContructAesDec(key, 0, 0)
	if err != nil {
		t.Fatal("AesEncrypt error")
	}

	origin := make([]byte, len(data))
	ad.CryptBlocks(origin, crypted)

	if !bytes.Equal(origin, data) {
		t.Fatal("error")
	}
}

func fillRandom(p []byte) {
	for i := 0; i < len(p); i += 7 {
		val := rand.Int63()
		for j := 0; i+j < len(p) && j < 7; j++ {
			p[i+j] = byte(val)
			val >>= 8
		}
	}
}
