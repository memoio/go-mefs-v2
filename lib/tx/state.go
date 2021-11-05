package tx

import (
	"github.com/fxamacker/cbor/v2"
	"github.com/memoio/go-mefs-v2/lib/types"
)

type TxMsgState struct {
	BlockID types.MsgID
	Height  uint64
	Status  ErrCode // return code
}

func (m *TxMsgState) Serialize() ([]byte, error) {
	return cbor.Marshal(m)
}

func (m *TxMsgState) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, m)
}
