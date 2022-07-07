package control

import (
	"bytes"
	"encoding/binary"
	"sync"

	"github.com/memoio/go-mefs-v2/lib/backend/wrap"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"golang.org/x/xerrors"
)

const (
	prefix = "restrict"
	eflag  = "enable"
)

type wl struct {
	sync.RWMutex
	enable bool
	list   map[uint64]struct{}
	ds     store.KVStore
}

func New(ds store.KVStore) *wl {
	w := &wl{
		ds:   wrap.NewKVStore(prefix, ds),
		list: make(map[uint64]struct{}),
	}

	val, err := w.ds.Get(store.NewKey(eflag))
	if err == nil && bytes.Equal(val, []byte(eflag)) {
		w.enable = true
	}

	w.ds.Iter([]byte(""), w.add)

	return w
}

func (w *wl) add(key, value []byte) error {
	if len(value) == 8 {
		w.list[binary.BigEndian.Uint64(value)] = struct{}{}
	}

	return nil
}

func (w *wl) Enable(ea bool) error {
	w.Lock()
	defer w.Unlock()
	w.enable = ea
	if ea {
		w.ds.Put(store.NewKey(eflag), []byte(eflag))
	} else {
		w.ds.Delete(store.NewKey(eflag))
	}

	return nil
}

func (w *wl) Stat() bool {
	w.RLock()
	defer w.RUnlock()
	return w.enable
}

func (w *wl) Add(uid uint64) error {
	w.Lock()
	defer w.Unlock()

	if !w.enable {
		return xerrors.Errorf("enable restrict first")
	}

	w.list[uid] = struct{}{}

	value := make([]byte, 8)
	binary.BigEndian.PutUint64(value, uid)
	w.ds.Put(store.NewKey(uid), value)

	return nil
}

func (w *wl) Delete(uid uint64) error {
	w.Lock()
	defer w.Unlock()

	if !w.enable {
		return xerrors.Errorf("enable restrict first")
	}

	delete(w.list, uid)
	w.ds.Delete(store.NewKey(uid))

	return nil
}

func (w *wl) Has(uid uint64) bool {
	w.RLock()
	defer w.RUnlock()

	if !w.enable {
		return true
	}

	_, has := w.list[uid]
	return has
}

func (w *wl) List() []uint64 {
	w.RLock()
	defer w.RUnlock()

	res := make([]uint64, 0, len(w.list))
	for key := range w.list {
		res = append(res, key)
	}
	return res
}
