package utils

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"golang.org/x/crypto/sha3"
)

// ToEthAddress returns an address using the SECP256K1 protocol.
// pubkey is 65 bytes
func ToEthAddress(pubkey []byte) []byte {
	if len(pubkey) == 65 {
		d := sha3.NewLegacyKeccak256()
		d.Write(pubkey[1:])
		payload := d.Sum(nil)
		return payload[12:]
	}

	return pubkey
}

func UintToBytes(v interface{}) []byte {
	typ := reflect.TypeOf(v).Kind()
	switch typ {
	case reflect.Uint64:
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, v.(uint64))
		return buf
	case reflect.Uint32:
		buf := make([]byte, 4)
		binary.BigEndian.PutUint32(buf, v.(uint32))
		return buf
	case reflect.Uint16:
		buf := make([]byte, 2)
		binary.BigEndian.PutUint16(buf, v.(uint16))
		return buf
	default:
		return nil
	}
}

func Disorder(array []interface{}) {
	var temp interface{}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := len(array) - 1; i >= 0; i-- {
		num := r.Intn(i + 1)
		temp = array[i]
		array[i] = array[num]
		array[num] = temp
	}
}

func DisorderUint(array []uint64) {
	var temp uint64
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := len(array) - 1; i >= 0; i-- {
		num := r.Intn(i + 1)
		temp = array[i]
		array[i] = array[num]
		array[num] = temp
	}
}

func LeftPadBytes(slice []byte, l int) []byte {
	if l <= len(slice) {
		return slice
	}

	padded := make([]byte, l)
	copy(padded[l-len(slice):], slice)

	return padded
}

func GetDirSize(path string) (uint64, error) {
	var size uint64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += uint64(info.Size())
		}
		return nil
	})
	return size, err
}

const (
	KiB = 1024
	MiB = 1048576
	GiB = 1073741824
	TiB = 1099511627776

	KB = 1e3
	MB = 1e6
	GB = 1e9
	TB = 1e12

	//SHOWTIME 用于输出给使用者
	SHOWTIME = "2006-01-02 15:04:05 MST"
)

// FormatBytes convert bytes to human readable string. Like 2 MiB, 64.2 KiB, 52 B
func FormatBytes(i int64) (result string) {
	switch {
	case i >= TiB:
		result = fmt.Sprintf("%.02f TiB", float64(i)/TiB)
	case i >= GiB:
		result = fmt.Sprintf("%.02f GiB", float64(i)/GiB)
	case i >= MiB:
		result = fmt.Sprintf("%.02f MiB", float64(i)/MiB)
	case i >= KiB:
		result = fmt.Sprintf("%.02f KiB", float64(i)/KiB)
	default:
		result = fmt.Sprintf("%d B", i)
	}
	return
}

// FormatBytesDec Convert bytes to base-10 human readable string. Like 2 MB, 64.2 KB, 52 B
func FormatBytesDec(i int64) (result string) {
	switch {
	case i >= TB:
		result = fmt.Sprintf("%.02f TB", float64(i)/TB)
	case i >= GB:
		result = fmt.Sprintf("%.02f GB", float64(i)/GB)
	case i >= MB:
		result = fmt.Sprintf("%.02f MB", float64(i)/MB)
	case i >= KB:
		result = fmt.Sprintf("%.02f KB", float64(i)/KB)
	default:
		result = fmt.Sprintf("%d B", i)
	}
	return
}

func Binomial(n, k uint64) uint64 {
	if n < 0 || k < 0 {
		panic("negative input")
	}
	if n < k {
		panic("n must no less than k")
	}
	// (n,k) = (n, n-k)
	if k > n/2 {
		k = n - k
	}
	b := uint64(1)
	for i := uint64(1); i <= k; i++ {
		b = (n - k + i) * b / i
	}
	return b
}

// n is total: data+parity chunks
// m is data chunks
func CalReliabilty(n, m int, avail float64) float64 {
	res := float64(0)
	for i := 0; i < m; i++ {
		c := Binomial(uint64(n), uint64(i))
		a := math.Pow(avail, float64(i))
		una := float64(1) - avail
		una = math.Pow(una, float64(n-i))
		res += (float64(c) * a * una)
	}

	return 1 - res
}
