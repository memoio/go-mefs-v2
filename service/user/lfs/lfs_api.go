package lfs

import "github.com/memoio/go-mefs-v2/api"

var _ api.ILfsService = (*lfs_API)(nil)

type lfs_API struct {
	*LfsService
}
