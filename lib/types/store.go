package types

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
	Close() error
}

type KVStore interface {
	Store
	GetNext(key []byte, bandwidth int) (uint64, error)
}

type FileStore interface {
	Store
	Stat() (*Statistics, error)
}
