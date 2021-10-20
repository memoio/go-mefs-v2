package segment

import (
	"encoding/binary"
	"errors"

	"github.com/memoio/go-mefs-v2/lib/crypto/pdp"
)

const (
	DefaultPrefixLen = 24
)

var (
	ErrDataLength = errors.New("data length is wrong")
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
	if len(data) < DefaultPrefixLen {
		return nil, 0, ErrDataLength
	}

	version := binary.BigEndian.Uint32(data[:4])
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

type BaseSegment struct {
	SegID SegmentID
	Data  []byte
}

func NewBaseSegment(data []byte, segID SegmentID) Segment {
	return &BaseSegment{
		Data:  data,
		SegID: segID,
	}
}

func (bs *BaseSegment) RawData() []byte {
	return bs.Data
}

func (bs *BaseSegment) SegData() ([]byte, error) {
	pre, preLen, err := DeserializePrefix(bs.Data)
	if err != nil {
		return nil, err
	}
	seg := make([]byte, pre.SegSize)
	copy(seg, bs.Data[preLen:])
	return seg, nil
}

func (bs *BaseSegment) Tag() ([]byte, error) {
	pre, preLen, err := DeserializePrefix(bs.Data)
	if err != nil {
		return nil, err
	}
	tag := make([]byte, pdp.TagMap[int(pre.TagFlag)])
	copy(tag, bs.Data[pre.SegSize+uint32(preLen):])
	return tag, nil
}

func (bs *BaseSegment) SegmentID() SegmentID {
	return bs.SegID
}

func (bs *BaseSegment) FsID() []byte {
	return bs.SegID.GetFsID()
}

func (bs *BaseSegment) BucketID() int64 {
	return bs.SegID.GetBucketID()
}

func (bs *BaseSegment) StripeID() int64 {
	return bs.SegID.GetStripeID()
}
func (bs *BaseSegment) ChunkID() uint32 {
	return bs.SegID.GetChunkID()
}
