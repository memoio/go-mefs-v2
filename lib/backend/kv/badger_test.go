package kv

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"testing"
)

func TestCreateDS(t *testing.T) {
	path, err := ioutil.TempDir(os.TempDir(), "testing_badger_")
	if err != nil {
		t.Fatal(err)
	}

	d, err := NewBadgerStore(path, nil)
	if err != nil {
		t.Fatal(err)
	}

	testKey := []byte("/test")
	testVal := []byte("aaaaa")

	err = d.Put(testKey, testVal)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 10; i++ {
		tkey := append(testKey, []byte(strconv.Itoa(i))...)
		err = d.Put(tkey, testVal)
		if err != nil {
			t.Fatal(err)
		}
	}

	fn := func(key, value []byte) error {
		fmt.Println(key)
		return nil
	}

	res := d.Iter(testKey, fn)
	t.Fatal(res)

	ok, err := d.Has(testKey)
	if err != nil {
		t.Fatal(err)
	}

	if !ok {
		t.Fatal("not have")
	}

	val, err := d.Get(testKey)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(val, testVal) {
		t.Fatal("not equal")
	}

	count, err := d.GetNext([]byte("testseq"), 100)
	if err != nil {
		t.Fatal(err)
	}

	count, err = d.GetNext([]byte("testseq"), 100)
	if err != nil {
		t.Fatal(err)
	}

	if count != 1 {
		t.Fatal(count)
	}

	t.Fatal(d.GetBadgerDB().Size())

	d.Close()
	os.RemoveAll(path)
}
