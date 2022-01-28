package tx

import (
	"github.com/fxamacker/cbor/v2"
	"github.com/memoio/go-mefs-v2/lib/types"
)

type MsgState struct {
	BlockID types.MsgID
	Height  uint64
	Status  Receipt
}

func (ms *MsgState) Serialize() ([]byte, error) {
	return cbor.Marshal(ms)
}

func (ms *MsgState) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, ms)
}
