package code

import (
	"golang.org/x/xerrors"

	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	pdpv2 "github.com/memoio/go-mefs-v2/lib/crypto/pdp/version2"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
)

const (
	RsPolicy         = 1
	MulPolicy        = 2
	CurrentVersion   = 1
	DefaultCrypt     = 1
	DefaultPrefixLen = 24
	DefaultTagFlag   = pdpcommon.PDPV2
	DefaultSegSize   = pdpv2.DefaultSegSize
)

type Codec interface {
	// name is fsID_bucketID_stripeID
	Encode(name segment.SegmentID, data []byte) ([][]byte, error)

	// name is fsID_bucketID_stripeID; if set, verify tag;
	// name can set to "" as fast mode
	Decode(name segment.SegmentID, stripe [][]byte) ([]byte, error)
	Recover(name segment.SegmentID, stripe [][]byte) error
	VerifyStripe(name segment.SegmentID, stripe [][]byte) (bool, int, error)
	// name is fsID_bucketID_stripeID_chunkID
	VerifyChunk(name segment.SegmentID, data []byte) (bool, error)
}

// DefaultBucketOptions is default bucket option
func DefaultBucketOptions() pb.BucketOption {
	return pb.BucketOption{
		Version:     1,
		Policy:      RsPolicy,
		DataCount:   5,
		ParityCount: 5,
		SegSize:     DefaultSegSize,
		TagFlag:     DefaultTagFlag,
	}
}

func (d *DataCoder) VerifyPrefix(pre *segment.Prefix) bool {
	if pre == nil || pre.Version != d.Version || pre.DataCount != d.DataCount || pre.ParityCount != d.ParityCount || pre.Policy != d.Policy || pre.SegSize != d.SegSize || pre.TagFlag != d.TagFlag {
		return false
	}

	return true
}

//VerifyChunkLength verify length of a chunk
func VerifyChunkLength(data []byte) error {
	if data == nil {
		return xerrors.New("data length is zero")
	}
	pre, preLen, err := segment.DeserializePrefix(data)
	if err != nil {
		return err
	}

	if preLen != pre.Size() {
		return xerrors.New("data length is wrong")
	}

	if pre.Version != 1 {
		return xerrors.Errorf("version %d is not supported", pre.Version)
	}

	if pre.DataCount == 0 {
		return xerrors.Errorf("policy is not supported")
	}

	tagLen, ok := pdpcommon.TagMap[int(pre.TagFlag)]
	if !ok {
		tagLen = 48
	}

	fragSize := int(pre.SegSize) + tagLen*int(2+(pre.ParityCount-1)/pre.DataCount) + pre.Size()

	if len(data) != fragSize {
		return xerrors.New("data length is wrong")
	}

	return nil
}

func Verify(k pdpcommon.KeySet, name segment.SegmentID, data []byte) bool {
	if len(data) == 0 || k == nil || k.PublicKey() == nil {
		return false
	}

	prefix, _, err := segment.DeserializePrefix(data[:DefaultPrefixLen])
	if err != nil || prefix.Version == 0 || prefix.DataCount == 0 {
		return false
	}

	d, err := NewDataCoderWithPrefix(k, prefix)
	if err != nil {
		return false
	}

	ok, err := d.VerifyChunk(name, data)
	if err != nil {
		return false
	}
	return ok
}

// Repair stripes
func Repair(keyset pdpcommon.KeySet, name segment.SegmentID, stripe [][]byte) ([][]byte, error) {
	var prefix *segment.Prefix
	var err error
	for _, s := range stripe {
		if len(s) >= DefaultPrefixLen {
			pre, _, err := segment.DeserializePrefix(s)
			if err != nil {
				return nil, err
			}

			prefix = pre
			break
		}
	}

	coder, err := NewDataCoderWithPrefix(keyset, prefix)
	if err != nil {
		return nil, err
	}

	err = coder.Recover(nil, stripe)
	if err != nil {
		return nil, err
	}

	return stripe, nil
}
