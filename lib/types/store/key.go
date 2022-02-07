package store

import (
	"bytes"
	"reflect"
	"strconv"

	"github.com/memoio/go-mefs-v2/lib/pb"
)

const (
	// KeyDelimiter seps kv key
	KeyDelimiter = "/"
)

// key is human readable
// type/key/...
func NewKey(vs ...interface{}) []byte {
	var b bytes.Buffer
	vlen := len(vs)
	for i, v := range vs {
		typ := reflect.TypeOf(v).Kind()
		switch typ {
		case reflect.Slice:
			val := v.([]byte)
			b.Write(val)
		case reflect.String:
			val := v.(string)
			b.Write([]byte(val))
		case reflect.Int:
			val := v.(int)
			b.Write([]byte(strconv.FormatInt(int64(val), 10)))
		case reflect.Int32:
			val1, ok := v.(pb.MetaType)
			if ok {
				b.Write([]byte(strconv.FormatInt(int64(val1), 10)))
			} else {
				val := v.(int32)
				b.Write([]byte(strconv.FormatInt(int64(val), 10)))
			}
		case reflect.Int64:
			val := v.(int64)
			b.Write([]byte(strconv.FormatInt(val, 10)))
		case reflect.Uint:
			val := v.(uint)
			b.Write([]byte(strconv.FormatUint(uint64(val), 10)))
		case reflect.Uint32:
			val := v.(uint32)
			b.Write([]byte(strconv.FormatUint(uint64(val), 10)))
		case reflect.Uint64:
			val := v.(uint64)
			b.Write([]byte(strconv.FormatUint(val, 10)))
		default:
			continue
		}

		if i < vlen-1 {
			b.Write([]byte(KeyDelimiter))
		}
	}

	return b.Bytes()
}
