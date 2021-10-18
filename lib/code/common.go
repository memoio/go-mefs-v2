package dataformat

import (
	"encoding/binary"
	"errors"

	"github.com/memoio/go-mefs-v2/lib/crypto/pdp"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	pdpv2 "github.com/memoio/go-mefs-v2/lib/crypto/pdp/version2"
	mpb "github.com/memoio/go-mefs-v2/lib/pb"
)

const (
	RsPolicy         = 1
	MulPolicy        = 2
	CurrentVersion   = 1
	DefaultCrypt     = 1
	DefaultPrefixLen = 24
	DefaultTagFlag   = pdp.PDPV1
	DefaultSegSize   = pdpv2.DefaultSegSize
)

var (
	ErrWrongCoder   = errors.New("coder is not supported")
	ErrWrongVersion = errors.New("version is not supported")
	ErrWrongTagFlag = errors.New("no such tag flag")
	ErrWrongPolicy  = errors.New("policy is not supported")
	ErrDataLength   = errors.New("data length is wrong")
	ErrDataBroken   = errors.New("datais broken")
	ErrRepairCrash  = errors.New("repair crash")
	ErrRecoverData  = errors.New("the recovered data is incorrect")
)

type Prefix struct {
	Version     uint32
	Policy      uint32
	DataCount   uint32
	ParityCount uint32
	TagFlag     uint32
	SegSize     uint32
}

func (p Prefix) Serialize() []byte {
	buf := make([]byte, DefaultPrefixLen)
	binary.BigEndian.PutUint32(buf[:4], uint32(p.Version))
	binary.BigEndian.PutUint32(buf[4:8], uint32(p.Policy))
	binary.BigEndian.PutUint32(buf[8:12], uint32(p.DataCount))
	binary.BigEndian.PutUint32(buf[12:16], uint32(p.ParityCount))
	binary.BigEndian.PutUint32(buf[16:20], uint32(p.TagFlag))
	binary.BigEndian.PutUint32(buf[20:24], uint32(p.SegSize))
	return buf
}

func (p Prefix) Size() int {
	return DefaultPrefixLen
}

func DeserializePrefix(data []byte) (*Prefix, int, error) {
	if len(data) < 4 {
		return nil, 0, ErrDataLength
	}
	version := binary.BigEndian.Uint32(data[:4])
	if version > 1 {
		return nil, 0, ErrWrongVersion
	}

	if len(data) < DefaultPrefixLen {
		return nil, 0, ErrDataLength
	}

	policy := binary.BigEndian.Uint32(data[4:8])
	dataCount := binary.BigEndian.Uint32(data[8:12])
	parityCount := binary.BigEndian.Uint32(data[12:16])
	tagFlag := binary.BigEndian.Uint32(data[16:20])
	segSize := binary.BigEndian.Uint32(data[20:24])
	return &Prefix{
		Version:     version,
		Policy:      policy,
		DataCount:   dataCount,
		ParityCount: parityCount,
		TagFlag:     tagFlag,
		SegSize:     segSize,
	}, DefaultPrefixLen, nil
}

// DefaultBucketOptions is default bucket option
func DefaultBucketOptions() *mpb.BucketOption {
	return &mpb.BucketOption{
		Version:     1,
		Policy:      RsPolicy,
		DataCount:   3,
		ParityCount: 2,
		SegSize:     DefaultSegSize,
		TagFlag:     DefaultTagFlag,
	}
}

// DefaultSuperBucketOptions is default supberbucket option
func DefaultSuperBucketOptions() *mpb.BucketOption {
	return &mpb.BucketOption{
		Version:     1,
		Policy:      MulPolicy,
		DataCount:   1,
		ParityCount: 2,
		SegSize:     DefaultSegSize,
		TagFlag:     DefaultTagFlag,
	}
}

func (d *DataCoder) VerifyPrefix(pre *Prefix) bool {
	if pre == nil || pre.Version != d.Version || pre.DataCount != d.DataCount || pre.ParityCount != d.ParityCount || pre.Policy != d.Policy || pre.SegSize != d.SegSize || pre.TagFlag != d.TagFlag {
		return false
	}

	return true
}

func (p Prefix) VerifyLength(size int) error {
	if p.Version != 1 {
		return ErrWrongVersion
	}

	if p.DataCount == 0 {
		return ErrWrongPolicy
	}

	tagLen, ok := pdp.TagMap[int(p.TagFlag)]
	if !ok {
		tagLen = 48
	}

	fragSize := int(p.SegSize) + tagLen*int(2+(p.ParityCount-1)/p.DataCount) + p.Size()

	if size != fragSize {
		return ErrDataLength
	}

	return nil
}

//VerifyChunkLength verify length of a chunk
func VerifyChunkLength(data []byte) error {
	if data == nil {
		return ErrDataLength
	}
	pre, preLen, err := DeserializePrefix(data)
	if err != nil {
		return err
	}

	if preLen != pre.Size() {
		return ErrDataLength
	}

	return pre.VerifyLength(len(data))
}

func Verify(k pdpcommon.KeySet, name string, data []byte) bool {
	if len(data) == 0 || k == nil || k.PublicKey() == nil {
		return false
	}

	prefix, _, err := DeserializePrefix(data[:DefaultPrefixLen])
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
func Repair(keyset pdpcommon.KeySet, name string, stripe [][]byte) ([][]byte, error) {
	var prefix *Prefix
	var err error
	for _, s := range stripe {
		if len(s) >= DefaultPrefixLen {
			pre, _, err := DeserializePrefix(s)
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

	err = coder.Recover("", stripe)
	if err != nil {
		return nil, err
	}

	return stripe, nil
}
