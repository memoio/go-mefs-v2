package segment

import (
	"encoding/binary"

	"golang.org/x/xerrors"

	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
)

const (
	DefaultPrefixLen = 24
)

var (
	ErrDataLength = xerrors.New("data length is wrong")
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
	segID SegmentID
	data  []byte
}

func NewBaseSegment(data []byte, segID SegmentID) Segment {
	return &BaseSegment{
		data:  data,
		segID: segID,
	}
}

func (bs *BaseSegment) SetID(segID SegmentID) {
	bs.segID = segID
}

func (bs *BaseSegment) SetData(data []byte) {
	bs.data = data
}

func (bs *BaseSegment) SegmentID() SegmentID {
	return bs.segID
}

func (bs *BaseSegment) Data() []byte {
	return bs.data
}

func (bs *BaseSegment) Content() ([]byte, error) {
	pre, preLen, err := DeserializePrefix(bs.data)
	if err != nil {
		return nil, err
	}
	seg := make([]byte, pre.SegSize)
	copy(seg, bs.data[preLen:])
	return seg, nil
}

func (bs *BaseSegment) Tags() ([][]byte, error) {
	pre, preLen, err := DeserializePrefix(bs.data)
	if err != nil {
		return nil, err
	}

	if pre.DataCount < 1 || pre.ParityCount < 1 {
		return nil, ErrDataLength
	}

	tagLen := pdpcommon.TagMap[int(pre.TagFlag)]
	tagCount := 2 + int((pre.ParityCount-1)/pre.DataCount)

	tag := make([][]byte, tagCount)
	for i := 0; i < tagCount; i++ {
		tag[i] = append(tag[i], bs.data[int(pre.SegSize)+preLen+i*tagLen:int(pre.SegSize)+preLen+(i+1)*tagLen]...)
	}

	return tag, nil
}

func (bs *BaseSegment) Serialize() ([]byte, error) {
	buf := make([]byte, 40+len(bs.data))
	copy(buf[:40], bs.segID.Bytes())
	copy(buf[40:], bs.data)

	return buf, nil
}

func (bs *BaseSegment) Deserialize(b []byte) error {
	if len(b) < 40 {
		return ErrDataLength
	}

	segID, err := FromBytes(b[:40])
	if err != nil {
		return err
	}

	_, _, err = DeserializePrefix(b[40:])
	if err != nil {
		return err
	}

	bs.segID = segID
	bs.data = b[40:]

	return nil
}
