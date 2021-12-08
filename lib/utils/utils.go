package utils

import (
	"encoding/binary"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/mitchellh/go-homedir"
	"golang.org/x/crypto/sha3"
	"golang.org/x/xerrors"
)

func GetMefsPath() (string, error) {
	mefsPath := "~/.memo"
	if os.Getenv("MEFS_PATH") != "" { //获取环境变量
		mefsPath = os.Getenv("MEFS_PATH")
	}
	mefsPath, err := homedir.Expand(mefsPath)
	if err != nil {
		return "", err
	}
	return mefsPath, nil
}

// ToEthAddress returns an address using the SECP256K1 protocol.
// pubkey is 65 bytes
func ToEthAddress(pubkey []byte) ([]byte, error) {
	if len(pubkey) != 65 {
		return nil, xerrors.New("length should be 65")
	}

	d := sha3.NewLegacyKeccak256()
	d.Write(pubkey[1:])
	payload := d.Sum(nil)
	return payload[12:], nil
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
