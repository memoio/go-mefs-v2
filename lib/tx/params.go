package tx

import (
	"github.com/fxamacker/cbor/v2"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
)

type BucketParams struct {
	pb.BucketOption
	BucketID uint64
}

func (bp *BucketParams) Serialize() ([]byte, error) {
	return cbor.Marshal(bp)
}

func (bp *BucketParams) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, bp)
}

type EpochParams struct {
	Seed []byte // hash pre
	Sig  types.MultiSignature
}
