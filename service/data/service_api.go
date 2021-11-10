package data

import "github.com/memoio/go-mefs-v2/api"

var _ api.IDataService = (*data_API)(nil)

type data_API struct {
	*dataService
}
