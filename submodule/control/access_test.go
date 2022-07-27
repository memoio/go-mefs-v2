package control

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/memoio/go-mefs-v2/lib/backend/kv"
	"github.com/memoio/go-mefs-v2/lib/backend/wrap"
)

func TestCreateAccess(t *testing.T) {
	path, err := ioutil.TempDir(os.TempDir(), "testing_access_")
	if err != nil {
		t.Fatal(err)
	}

	d, err := kv.NewBadgerStore(path, nil)
	if err != nil {
		t.Fatal(err)
	}

	w := New(wrap.NewKVStore("tas", d))

	l := w.List()
	t.Log(l)

	w.Enable(true)

	for i := uint64(11); i < 30; i++ {
		w.Add(i)
	}

	l = w.List()
	t.Log(l)

	t.Log(w.Has(15))

	w.Delete(15)

	t.Log(w.Has(15))

	w.Enable(false)

	t.Log(w.Has(15))

	l = w.List()
	t.Log(l)

	d.Close()

	t.Fatal("end")
	os.RemoveAll(path)
}
