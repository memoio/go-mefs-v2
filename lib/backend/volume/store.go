package volume

import "errors"

var (
	ErrNotFound    = errors.New("key not found")
	ErrDataLength  = errors.New("data length is incorrect")
	ErrDirExist    = errors.New("dir exist")
	ErrEmptyConfig = errors.New("config is nil")
	ErrClosed      = errors.New("volume filestore closed")
)
