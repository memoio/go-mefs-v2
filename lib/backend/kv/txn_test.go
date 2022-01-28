package kv

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"
)

var (
	testKey = []byte("/test")
	testVal = []byte("aaaaa")
)

func testCommit(t *testing.T, p bool, dp *string) {
	path, err := ioutil.TempDir(os.TempDir(), "testing_txn_")
	if err != nil {
		t.Fatal(err)
	}
	*dp = path

	d, err := NewBadgerStore(path, nil)
	if err != nil {
		t.Fatal(err)
	}

	// test key and value
	testVal2 := []byte("bbbbb")
	testVal3 := []byte("ccccc")

	if p {
		defer func() {
			err := recover()
			t.Log(err)
			val, _ := d.Get(testKey)
			if !bytes.Equal(val, testVal) {
				t.Fatalf("not equal, get %s, expect %s\n", val, testVal)
			}
			t.Log(val)
			t.Log(testVal)
			d.Close()
		}()
	}

	// d
	err = d.Put(testKey, testVal)
	if err != nil {
		t.Fatal(err)
	}

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

	// txn
	// new a txn
	txn, err := d.NewTxnStore(true)
	if err != nil {
		t.Fatal(err)
	}
	// get
	val, err = txn.Get(testKey)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(val, testVal) {
		t.Fatal("Txn not equal")
	}

	// put
	err = txn.Put(testKey, testVal2)
	if err != nil {
		t.Fatal(err)
	}
	val, _ = txn.Get(testKey)
	if !bytes.Equal(val, testVal2) {
		t.Fatal("value in txn is not properly put")
	}
	val, _ = d.Get(testKey)
	if !bytes.Equal(val, testVal) {
		t.Fatal("origin value has been modified")
	}

	// put2
	err = txn.Put(testKey, testVal3)
	if err != nil {
		t.Fatal(err)
	}
	val, _ = txn.Get(testKey)
	if !bytes.Equal(val, testVal3) {
		t.Fatal("value in txn is not properly put")
	}
	val, _ = d.Get(testKey)
	if !bytes.Equal(val, testVal) {
		t.Fatal("origin value has been modified")
	}

	if p {
		panic("Trigger panic before commit")
	}

	// commit
	err = txn.Commit()
	if err != nil {
		t.Fatal(err)
	}
	val, _ = d.Get(testKey)
	if !bytes.Equal(val, testVal3) {
		t.Fatal("something wrong in txn commit")
	}

	d.Close()
	os.RemoveAll(path)
}

func testReopen(t *testing.T, path string) {
	d, err := NewBadgerStore(path, nil)
	if err != nil {
		t.Fatal(err)
	}

	val, err := d.Get(testKey)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(val, testVal) {
		t.Fatalf("not equal, get %s, expect %s\n", val, testVal)
	}
	d.Close()
	os.RemoveAll(path)
}

func TestCommit(t *testing.T) {
	var p string
	testCommit(t, false, &p)
}

func TestPanic(t *testing.T) {
	var p string
	testCommit(t, true, &p)
	testReopen(t, p)
}
