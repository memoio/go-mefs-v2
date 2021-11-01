package segment

import (
	"encoding/binary"
	"errors"

	"github.com/mr-tron/base58/base58"
)

var (
	ErrWrongType      = errors.New("mismatch type")
	ErrIllegalKey     = errors.New("this key is illegal")
	ErrWrongKeyLength = errors.New("this key's length is wrong")
	ErrIllegalValue   = errors.New("this metavalue is illegal")
)

const SEGMENTID_LEN = 40
const FSID_LEN = 20

// Segmentid for handle Segment
// SegmentID is fsid+bucketid+stripeid+chunkid (20+8+8+4)
type BaseSegmentID struct {
	buf []byte // 40 Byte
}

func (bm *BaseSegmentID) GetFsID() []byte {
	return bm.buf[:FSID_LEN]
}

func (bm *BaseSegmentID) GetBucketID() int64 {
	return int64(binary.BigEndian.Uint64(bm.buf[FSID_LEN : FSID_LEN+8]))
}

func (bm *BaseSegmentID) GetStripeID() int64 {
	return int64(binary.BigEndian.Uint64(bm.buf[FSID_LEN+8 : FSID_LEN+16]))
}

func (bm *BaseSegmentID) GetChunkID() uint32 {
	return binary.BigEndian.Uint32(bm.buf[FSID_LEN+16 : SEGMENTID_LEN])
}

func (bm *BaseSegmentID) SetBucketID(bid int64) {
	binary.BigEndian.PutUint64(bm.buf[FSID_LEN:FSID_LEN+8], uint64(bid))
}

func (bm *BaseSegmentID) SetStripeID(sid int64) {
	binary.BigEndian.PutUint64(bm.buf[FSID_LEN+8:FSID_LEN+16], uint64(sid))
}

func (bm *BaseSegmentID) SetChunkID(cid uint32) {
	binary.BigEndian.PutUint32(bm.buf[FSID_LEN+16:SEGMENTID_LEN], cid)
}

func (bm *BaseSegmentID) Bytes() []byte {
	return bm.buf
}

// ToString 将SegmentID结构体转换成字符串格式，进行传输
func (bm *BaseSegmentID) String() string {
	return base58.Encode(bm.buf)
}

// 不包含fsID
func (bm *BaseSegmentID) IndexBytes() []byte {
	return bm.buf[FSID_LEN:SEGMENTID_LEN]
}

// 不包含fsID
func (bm *BaseSegmentID) IndexString() string {
	return base58.Encode(bm.IndexBytes())
}

func NewSegmentID(fid []byte, bid, sid int64, cid uint32) (SegmentID, error) {
	if len(fid) != FSID_LEN {
		return nil, ErrWrongKeyLength
	}

	segID := make([]byte, SEGMENTID_LEN)
	copy(segID[:FSID_LEN], fid)

	binary.BigEndian.PutUint64(segID[FSID_LEN:FSID_LEN+8], uint64(bid))
	binary.BigEndian.PutUint64(segID[FSID_LEN+8:FSID_LEN+16], uint64(sid))
	binary.BigEndian.PutUint32(segID[FSID_LEN+16:SEGMENTID_LEN], cid)

	return &BaseSegmentID{buf: segID}, nil
}

//NewBlockFromString convert string to blockmeta
func NewSegmentIDFromString(key string) (SegmentID, error) {
	buf, err := base58.Decode(key)
	if err != nil {
		return nil, err
	}

	if len(buf) != SEGMENTID_LEN {
		return nil, ErrWrongKeyLength
	}

	return &BaseSegmentID{buf: buf}, nil
}
