package data

import "github.com/memoio/go-mefs-v2/api"

// wrap local store, cache and net

var _ api.IDataService = (*dataAPI)(nil)

type dataAPI struct {
	*dataService
}
