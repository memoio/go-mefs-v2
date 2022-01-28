package simplefs

import (
	"bytes"
	"os"
	"path"
	"testing"
)

func TestPath(t *testing.T) {
	p := "abd/abc/abc"

	t.Log(path.Base(p))
	t.Log(path.Dir(p))

	t.Log(path.Join("~", path.Dir(p)))

	err := os.MkdirAll(path.Join("/home/fjt", path.Dir(p)), 0755)
	t.Fatal(err)

	t.Fatal("finish")
}

func TestSimplefs(t *testing.T) {
	sfs, err := NewSimpleFs("/home/fjt/test/sfs")
	if err != nil {
		t.Fatal(err)
	}

	key := []byte("test/aa")
	val := []byte("test")

	err = sfs.Put(key, val)
	if err != nil {
		t.Fatal(err)
	}

	res, err := sfs.Get(key)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(val, res) {
		t.Fatal("not equal")
	}

	t.Fatal("finish")
}
