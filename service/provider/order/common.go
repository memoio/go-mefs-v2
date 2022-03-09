package order

import (
	logging "github.com/memoio/go-mefs-v2/lib/log"
)

var logger = logging.Logger("pro-order")

const (
	orderMaxSize = 100 * 24 * 3600 * 10 * 1024 * 1024 * 1024 // 100 day*10GB
	seqMaxSize   = 1024 * 1024 * 1024                        // 1 GB
)
