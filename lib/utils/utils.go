package utils

import (
	"encoding/binary"
	"math/rand"
	"os"
	"reflect"
	"time"

	"github.com/mitchellh/go-homedir"
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
