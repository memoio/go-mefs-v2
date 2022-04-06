package cachestore

import (
	lru "github.com/hashicorp/golang-lru"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

var _ store.KVStore = (*cstore)(nil)

type cstore struct {
	db store.KVStore

	cache *lru.ARCCache
}

func NewCacheStore(db store.KVStore) store.KVStore {
	cache, _ := lru.NewARC(1024)
	return &cstore{
		db,
		cache,
	}
}

// todo add cache ops
func (c *cstore) Put(key, value []byte) error {
	return c.db.Put(key, value)
}

func (c *cstore) Get(key []byte) ([]byte, error) {
	return c.db.Get(key)
}

func (c *cstore) Has(key []byte) (bool, error) {

	return c.db.Has(key)
}

func (c *cstore) Delete(key []byte) error {
	return c.db.Delete(key)
}

func (c *cstore) GetNext(key []byte, band int) (uint64, error) {
	return c.db.GetNext(key, band)
}

func (c *cstore) Iter(prefix []byte, fn func(k, v []byte) error) int64 {
	return c.db.Iter(prefix, fn)
}
func (c *cstore) IterKeys(prefix []byte, fn func(k []byte) error) int64 {
	return c.db.IterKeys(prefix, fn)
}

func (c *cstore) NewTxnStore(update bool) (store.TxnStore, error) {
	return c.db.NewTxnStore(update)
}

func (c *cstore) Size() store.DiskStats {
	return c.db.Size()
}

func (c *cstore) Sync() error {
	return c.db.Sync()
}

func (c *cstore) Close() error {
	return c.db.Close()
}
