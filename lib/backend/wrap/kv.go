package wrap

import "github.com/memoio/go-mefs-v2/lib/types/store"

var _ store.KVStore = (*kv)(nil)

type kv struct {
	db     store.KVStore
	prefix string
}

func NewKVStore(prefix string, db store.KVStore) store.KVStore {
	return &kv{
		db,
		prefix,
	}
}

func (w *kv) Put(key, value []byte) error {
	key = w.convertKey(key)
	return w.db.Put(key, value)
}

func (w *kv) Get(key []byte) ([]byte, error) {
	key = w.convertKey(key)
	return w.db.Get(key)
}

func (w *kv) Has(key []byte) (bool, error) {
	key = w.convertKey(key)
	return w.db.Has(key)
}

func (w *kv) Delete(key []byte) error {
	key = w.convertKey(key)
	return w.db.Delete(key)
}

func (w *kv) GetNext(key []byte, band int) (uint64, error) {
	key = w.convertKey(key)
	return w.db.GetNext(key, band)
}

func (w *kv) Iter(prefix []byte, fn func(k, v []byte) error) int64 {
	return w.db.Iter(prefix, fn)
}
func (w *kv) IterKeys(prefix []byte, fn func(k []byte) error) int64 {
	prefix = w.convertKey(prefix)
	return w.db.IterKeys(prefix, fn)
}

func (w *kv) NewTxnStore(update bool) (store.TxnStore, error) {
	return w.db.NewTxnStore(update)
}

func (w *kv) Size() store.DiskStats {
	return w.db.Size()
}

func (w *kv) Sync() error {
	return w.db.Sync()
}

func (w *kv) Close() error {
	return w.db.Close()
}

func (w *kv) convertKey(key []byte) []byte {
	for len(key) > 0 {
		if key[0] == '/' {
			if len(key) == 1 {
				return nil
			}
			key = key[1:]
		} else {
			break
		}
	}

	newKey := make([]byte, 0, len(w.prefix)+len(key)+1)
	newKey = append(newKey, []byte(w.prefix+"/")...)

	newKey = append(newKey, key...)
	return newKey
}
