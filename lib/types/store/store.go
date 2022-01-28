package store

type Statistics struct {
	Count       uint64 // number of needles
	Size        uint64 // total size used > content.Size()
	ContentSize uint64
}

type Store interface {
	Put(key, value []byte) error
	Get(key []byte) ([]byte, error)
	Has(key []byte) (bool, error)
	Delete(key []byte) error
	Size() int64
	Close() error
}

type KVStore interface {
	Store

	GetNext(key []byte, bandwidth int) (uint64, error)
	Iter(prefix []byte, fn func(k, v []byte) error) int64
	IterKeys(prefix []byte, fn func(k []byte) error) int64

	Sync() error

	NewTxnStore(bool) (TxnStore, error)
}

type TxnStore interface {
	Store
	Commit() error
	Discard()
}

type FileStore interface {
	Store
}
