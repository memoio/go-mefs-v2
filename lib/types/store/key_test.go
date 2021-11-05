package store

import (
	"testing"

	"github.com/memoio/go-mefs-v2/lib/pb"
)

func TestMetaKey(t *testing.T) {
	v1 := "hello1"
	v2 := []byte("ok2")
	v3 := int(-3)
	v4 := uint32(444444444)
	nkey := NewKey(pb.MetaType_TX_MessageKey, v1, v2, v3, v4)
	t.Fatal(string(nkey))
}
