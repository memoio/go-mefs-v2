package smt

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"

	"github.com/memoio/go-mefs-v2/lib/backend/kv"
)

func TestValueStore(t *testing.T) {
	path, err := ioutil.TempDir(os.TempDir(), "testing_vs_")
	if err != nil {
		t.Fatal(err)
	}

	d, err := kv.NewBadgerStore(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		d.Close()
		os.RemoveAll(path)
	}()

	vs := &ValueStore{d}

	// put 5 times
	for i := 0; i < 5; i++ {
		if err = vs.Put(testKey, testVal); err != nil {
			t.Fatal(err)
		}
	}
	// delete 4 times
	for i := 0; i < 4; i++ {
		if err = vs.Delete(testKey); err != nil {
			t.Fatal(err)
		}
	}
	// get value
	if getVal, err := vs.Get(testKey); err != nil {
		t.Fatal(err)
	} else {
		if !bytes.Equal(getVal, testVal) {
			t.Fatalf("Get value: %s, but actual value: %s\n", getVal, testVal)
		}
	}
	// delete and get again
	if err = vs.Delete(testKey); err != nil {
		t.Fatal(err)
	}
	if ok, err := vs.Has(testKey); err != nil {
		t.Fatal(err)
	} else {
		if ok {
			t.Fatal("value still exists after final deletion")
		}
	}
}
