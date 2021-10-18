package volume

import "errors"

var (
	ErrNotFound    = errors.New("key not found")
	ErrDataLength  = errors.New("data length is incorrect")
	ErrDirExist    = errors.New("dir exist")
	ErrEmptyConfig = errors.New("config is nil")
	ErrClosed      = errors.New("volume filestore closed")
)

type Statistics struct {
	Count       uint64 // number of needles
	Size        uint64 // total size used > content.Size()
	ContentSize uint64
}

type FileStore interface {
	Put(key string, value []byte) error
	Get(key string) ([]byte, error)
	Delete(key string) error
	Has(key string) (bool, error)
	Stat() (*Statistics, error)
	Close() error
}
