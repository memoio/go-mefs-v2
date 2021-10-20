package volume

import (
	"bytes"
	"os"
	"strconv"
	"testing"

	"github.com/memoio/go-mefs-v2/lib/backend/kv"
)

var basePath = "test_weed_base"

var testWeedDir1 = "/tmp/test_weed1"
var testWeedDir2 = "/tmp/test_weed2"
var idxDir = "/tmp/test_weed_idx"
var badgerDir = "/tmp/test_weed_badger"

func TestFileConfig(t *testing.T) {
	testConfig(t)
}

func testConfig(t *testing.T) *Config {
	c := DefaultConfig(idxDir)
	err := c.AddPath(testWeedDir1)
	if err != nil {
		t.Fatal(err)
	}

	err = c.AddPath(testWeedDir2)
	if err != nil {
		t.Fatal(err)
	}

	err = StoreConfig(c)
	if err != nil {
		t.Fatal(err)
	}

	newc, err := GetConfig()
	if err != nil {
		t.Fatal(err)
	}

	if newc.IdxFolder != c.IdxFolder || newc.Dirnames[0] != c.Dirnames[0] || newc.Dirnames[1] != c.Dirnames[1] {
		t.Fatal("not equal")
	}

	return c
}

func TestFileStore(t *testing.T) {

	os.Mkdir(badgerDir, 0755)
	d, err := kv.NewBadgerStore(badgerDir, nil)
	if err != nil {
		t.Fatal(err)
	}

	defer d.Close()

	cfg := testConfig(t)

	defer func() {
		//os.RemoveAll(badgerDir)
		//os.RemoveAll(testWeedDir)
	}()

	//cfg := DefaultConfig(idxDir, testWeedDir1)
	//cfg.AddPath(testWeedDir2)

	wd, err := NewWeed(cfg, d)
	if err != nil {
		t.Fatal(err)
	}

	defer wd.Close()

	testkey := []byte("testKey")
	testValue := []byte("test")

	err = wd.Put(testkey, testValue)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 300000; i++ {
		tkey := append(testkey, []byte(strconv.Itoa(i))...)
		err := wd.Put(tkey, testValue)
		if err != nil {
			t.Fatal(err)
		}
	}

	wd.Stat()

	for i := 0; i < 300000; i++ {
		tkey := append(testkey, []byte(strconv.Itoa(i))...)
		err := wd.Delete(tkey)
		if err != nil {
			t.Fatal(err)
		}
	}

	for i := 0; i < 300000; i++ {
		tkey := append(testkey, []byte(strconv.Itoa(i))...)
		has, err := wd.Has(tkey)
		if err == nil || has {
			t.Fatal("delete fail")
		}
	}

	wd.Stat()

	has, err := wd.Has(testkey)
	if err != nil {
		t.Fatal(err)
	}

	if !has {
		t.Fatal("not found")
	}

	val, err := wd.Get(testkey)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(testValue, val) {
		t.Fatal("not equal")
	}
	wd.(*Weed).GetFileMeta(toString(testkey), false)

	wd.(*Weed).GarbageCollect(true)
	d.CollectGarbage()
	wd.Stat()
}
