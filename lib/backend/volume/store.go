package volume

import "golang.org/x/xerrors"

var (
	ErrNotFound    = xerrors.New("key not found")
	ErrDataLength  = xerrors.New("data length is incorrect")
	ErrDirExist    = xerrors.New("dir exist")
	ErrEmptyConfig = xerrors.New("config is nil")
	ErrClosed      = xerrors.New("volume filestore closed")
)
