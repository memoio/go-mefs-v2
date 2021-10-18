package kv

type Store interface {
	Put(key, value []byte) error
	Get(key []byte) ([]byte, error)
	Delete(key []byte) error
	GetNext(key []byte, bandwidth int) (uint64, error)
	Close() error
}
