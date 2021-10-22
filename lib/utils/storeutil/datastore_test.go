package storeutil

import (
	"strconv"
	"testing"

	pds "github.com/ipfs/go-datastore"
	pdsq "github.com/ipfs/go-datastore/query"
	"github.com/memoio/go-mefs-v2/lib/backend/kv"
)

func TestDataStore(t *testing.T) {
	bs, err := kv.NewBadgerStore("/home/fjt/test/ds", &kv.DefaultOptions)
	if err != nil {
		t.Fatal(err)
	}

	ds, err := NewDatastore("abc", bs)
	if err != nil {
		t.Fatal(err)
	}

	testKey := pds.NewKey("test")
	testVal := []byte("aaaaa")

	err = ds.Put(testKey, testVal)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 10; i++ {
		tkey := pds.NewKey(testKey.String() + strconv.Itoa(i))
		err = ds.Put(tkey, testVal)
		if err != nil {
			t.Fatal(err)
		}
	}

	q := pdsq.Query{
		Prefix:   "/test",
		KeysOnly: false,
	}
	res, err := ds.Query(q)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("here1")

	for {
		e, ok := res.NextSync()
		if !ok {
			t.Log("here")
			break
		}

		t.Log(e.Entry)
	}
	t.Fatal("finish")
}
