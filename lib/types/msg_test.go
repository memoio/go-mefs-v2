package types

import (
	"testing"
)

func TestMsg(t *testing.T) {
	data := []byte("test")
	res := NewMsgID(data)
	res2 := NewMsgID(res.Bytes())
	t.Log(res.Equal(res2))

	t.Log(res.String())

	nid, err := FromString(res.String())
	if err != nil {
		t.Fatal(err)
	}

	t.Log(nid.String())

	nnid, err := FromBytes(nid.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	t.Log(nnid.String())

	t.Fatal(res.String(), len(res.Bytes()), len(res.String()))
}
