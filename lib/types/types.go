package types

import (
	"errors"
	"strconv"
	"strings"
)

const (
	// KeyDelimiter seps kv key
	KeyDelimiter = "/"
	// SegDelimiter seps segment name
	SegDelimiter = "_"
	// 32进制
	Radix = 32
)

var (
	ErrHexString = errors.New("string format is not right")
	ErrLength    = errors.New("length is illegal")
)

func ToHexString(sep string, id ...uint64) string {
	if len(id) == 0 {
		return ""
	}

	elems := make([]string, len(id))
	for i, val := range id {
		// 16进制
		elems[i] = strconv.FormatUint(val, Radix)
	}

	return strings.Join(elems, sep)
}

func DecodeHexString(hexStr, sep string) ([]uint64, error) {
	s := strings.Split(hexStr, sep)
	if len(s) == 0 {
		return nil, ErrHexString
	}

	res := make([]uint64, len(s))
	for i, str := range s {
		val, err := strconv.ParseUint(str, Radix, 64)
		if err != nil {
			return nil, err
		}
		res[i] = val
	}

	return res, nil
}
