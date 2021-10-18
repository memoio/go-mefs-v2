package types

import (
	"bytes"
	"sort"

	"github.com/dgraph-io/badger/v2"
	mtdb "github.com/iden3/go-iden3-core/db"
	mt "github.com/iden3/go-iden3-core/merkletree"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

type levelDB struct {
	db *leveldb.DB
}

func NewLevelDB(path string) (*levelDB, error) {
	opts := &opt.Options{
		BlockCacheCapacity:            8 * 1024 * 1024, // default value is 8MiB
		WriteBuffer:                   4 * 1024 * 1024, // default value is 4MiB
		CompactionTableSizeMultiplier: 10,              // default value is 1
	}

	db, err := leveldb.OpenFile(path, opts)
	if err != nil {
		return nil, err
	}

	return &levelDB{db}, nil
}

type MTStorageTx struct {
	s *badger.Txn
}

func (tx *MTStorageTx) Get(key []byte) ([]byte, error) {
	item, err := tx.s.Get(key)
	if err != nil {
		return nil, err
	}
	return item.ValueCopy(nil)
}

func (tx *MTStorageTx) Put(key []byte, value []byte) {
	tx.s.Set(key, value)
}

func (tx *MTStorageTx) Commit() error {
	return tx.s.Commit()
}

func (tx *MTStorageTx) Add(mtdb.Tx) {
}

func (tx *MTStorageTx) Delete(key []byte) error {
	return tx.s.Delete(key)
}

func (tx *MTStorageTx) Close() {
	tx.s.Discard()
}

type mtree struct {
	*mt.MerkleTree
}

func NewMTree(path string) (*mtree, error) {
	s, err := NewMTStorage(path)
	if err != nil {
		return nil, err
	}
	tr, err := mt.NewMerkleTree(s, 256)
	if err != nil {
		return nil, err
	}

	return &mtree{tr}, nil
}

type MTStorage struct {
	db     *badger.DB
	prefix []byte
}

func NewMTStorage(path string) (*MTStorage, error) {
	ops := badger.DefaultOptions(path)
	ops.WithKeepL0InMemory(true)
	db, err := badger.Open(ops)
	if err != nil {
		return nil, err
	}

	prefix := []byte("mt")

	return &MTStorage{db, prefix}, nil
}

func (m *MTStorage) Info() string {
	return "badger"
}

func (m *MTStorage) Close() {
	m.db.Close()
}

func (m *MTStorage) WithPrefix(prefix []byte) mtdb.Storage {
	var b bytes.Buffer
	b.Write(m.prefix)
	b.Write(prefix)
	return &MTStorage{m.db, b.Bytes()}
}

func (m *MTStorage) NewTx() (mtdb.Tx, error) {
	txn := m.db.NewTransaction(true)
	return &MTStorageTx{txn}, nil
}

// Get retreives a value from a key in the mt.Lvl
func (m *MTStorage) Get(key []byte) ([]byte, error) {
	var val []byte
	err := m.db.View(func(txn *badger.Txn) error {
		switch item, err := txn.Get(key); err {
		case nil:
			val, err = item.ValueCopy(nil)
			return err
		default:
			return err
		}
	})
	if err != nil {
		return nil, err
	}
	return val, nil
}

func clone(b0 []byte) []byte {
	b1 := make([]byte, len(b0))
	copy(b1, b0)
	return b1
}

func (m *MTStorage) List(limit int) ([]mtdb.KV, error) {
	ret := []mtdb.KV{}
	err := m.Iterate(func(key []byte, value []byte) (bool, error) {
		ret = append(ret, mtdb.KV{
			K: clone(key),
			V: clone(value),
		})
		if len(ret) == limit {
			return false, nil
		}
		return true, nil
	})
	return ret, err
}

func (m *MTStorage) Iterate(f func([]byte, []byte) (bool, error)) error {
	kvs := make([]mtdb.KV, 0)
	m.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek(m.prefix); it.ValidForPrefix(m.prefix); it.Next() {
			item := it.Item()
			k := item.Key()
			v, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}

			kvs = append(kvs, mtdb.KV{
				K: k,
				V: v,
			})
		}
		return nil
	})

	sort.SliceStable(kvs, func(i, j int) bool { return bytes.Compare(kvs[i].K, kvs[j].K) < 0 })
	for _, kv := range kvs {
		if cont, err := f(kv.K, kv.V); err != nil {
			return err
		} else if !cont {
			break
		}
	}
	return nil
}

// ====================================state badger=============================================
