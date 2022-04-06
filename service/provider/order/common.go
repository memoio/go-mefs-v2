package order

import (
	"github.com/fxamacker/cbor/v2"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/types"
)

var logger = logging.Logger("pro-order")

const (
	orderMaxSize = 10 * 1024 * 1024 * 1024 * 1024 // 10 TB
	seqMaxSize   = 1024 * 1024 * 1024 * 1024      // 1 TB
)

type DataInfo struct {
	types.OrderPayInfo
	Received       uint64
	ConfirmedNonce uint64 // nonce on data chain
	SubNonce       uint64 // nonce on data chain
}

func (di *DataInfo) Serialize() ([]byte, error) {
	return cbor.Marshal(di)
}

func (di *DataInfo) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, di)
}
