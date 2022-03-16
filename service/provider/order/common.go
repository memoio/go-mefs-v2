package order

import (
	logging "github.com/memoio/go-mefs-v2/lib/log"
)

var logger = logging.Logger("pro-order")

const (
	orderMaxSize = 10 * 1024 * 1024 * 1024 * 1024 // 10 TB
	seqMaxSize   = 1024 * 1024 * 1024 * 1024      // 1 TB
)
