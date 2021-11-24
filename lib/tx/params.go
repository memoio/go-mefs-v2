package tx

import (
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
)

type BucketParams struct {
	pb.BucketOption
	BucketID uint64
}

type EpochParams struct {
	Seed []byte // hash pre
	Sig  types.MultiSignature
}
