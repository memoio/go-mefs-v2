package segment

import (
	"encoding/binary"
	"strconv"

	"github.com/mr-tron/base58/base58"
	"golang.org/x/xerrors"
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

func (bm *BaseSegmentID) GetBucketID() uint64 {
	return binary.BigEndian.Uint64(bm.buf[FSID_LEN : FSID_LEN+8])
}

func (bm *BaseSegmentID) GetStripeID() uint64 {
	return binary.BigEndian.Uint64(bm.buf[FSID_LEN+8 : FSID_LEN+16])
}

func (bm *BaseSegmentID) GetChunkID() uint32 {
	return binary.BigEndian.Uint32(bm.buf[FSID_LEN+16 : SEGMENTID_LEN])
}

func (bm *BaseSegmentID) SetBucketID(bid uint64) {
	binary.BigEndian.PutUint64(bm.buf[FSID_LEN:FSID_LEN+8], bid)
}

func (bm *BaseSegmentID) SetStripeID(sid uint64) {
	binary.BigEndian.PutUint64(bm.buf[FSID_LEN+8:FSID_LEN+16], sid)
}

func (bm *BaseSegmentID) SetChunkID(cid uint32) {
	binary.BigEndian.PutUint32(bm.buf[FSID_LEN+16:SEGMENTID_LEN], cid)
}

func (bm *BaseSegmentID) Bytes() []byte {
	res := make([]byte, len(bm.buf))
	copy(res, bm.buf)
	return res
}

// ToString used in trans
func (bm *BaseSegmentID) ToString() string {
	return base58.Encode(bm.buf)
}

// used in print or output
func (bm *BaseSegmentID) String() string {
	return base58.Encode(bm.GetFsID()) + "_" + strconv.FormatUint(bm.GetBucketID(), 10) + "_" + strconv.FormatUint(bm.GetStripeID(), 10) + "_" + strconv.FormatUint(uint64(bm.GetChunkID()), 10)
}

func (bm *BaseSegmentID) ShortBytes() []byte {
	return bm.buf[FSID_LEN:SEGMENTID_LEN]
}

func (bm *BaseSegmentID) ShortString() string {
	return strconv.FormatUint(bm.GetBucketID(), 10) + "_" + strconv.FormatUint(bm.GetStripeID(), 10) + "_" + strconv.FormatUint(uint64(bm.GetChunkID()), 10)
}

func NewSegmentID(fid []byte, bid, sid uint64, cid uint32) (SegmentID, error) {
	if len(fid) != FSID_LEN {
		return nil, xerrors.New("segment length is wrong")
	}

	segID := make([]byte, SEGMENTID_LEN)
	copy(segID[:FSID_LEN], fid)

	binary.BigEndian.PutUint64(segID[FSID_LEN:FSID_LEN+8], bid)
	binary.BigEndian.PutUint64(segID[FSID_LEN+8:FSID_LEN+16], sid)
	binary.BigEndian.PutUint32(segID[FSID_LEN+16:SEGMENTID_LEN], cid)

	return &BaseSegmentID{buf: segID}, nil
}

// FromString convert string to segmentID
func FromString(key string) (SegmentID, error) {
	buf, err := base58.Decode(key)
	if err != nil {
		return nil, err
	}

	return FromBytes(buf)
}

// FromBytes convert bytes to segmentID
func FromBytes(b []byte) (SegmentID, error) {
	if len(b) != SEGMENTID_LEN {
		return nil, xerrors.New("segment length is wrong")
	}

	return &BaseSegmentID{buf: b}, nil
}

func CreateSegmentID(fid []byte, bid, sid uint64, cid uint32) []byte {
	segID := make([]byte, SEGMENTID_LEN)
	copy(segID[:FSID_LEN], fid)

	binary.BigEndian.PutUint64(segID[FSID_LEN:FSID_LEN+8], bid)
	binary.BigEndian.PutUint64(segID[FSID_LEN+8:FSID_LEN+16], sid)
	binary.BigEndian.PutUint32(segID[FSID_LEN+16:SEGMENTID_LEN], cid)

	return segID
}
