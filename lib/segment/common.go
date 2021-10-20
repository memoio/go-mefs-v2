package segment

type SegmentID interface {
	GetFsID() []byte
	GetBucketID() int64
	GetStripeID() int64
	GetChunkID() uint32

	SetBucketID(sID int64)
	SetStripeID(sID int64)
	SetChunkID(cID uint32)

	Bytes() []byte
	String() string

	IndexBytes() []byte
	IndexString() string
}

type Segment interface {
	SetID(SegmentID)
	SetData([]byte)
	RawData() []byte
	Tag() ([]byte, error)
	SegData() ([]byte, error)
	SegmentID() SegmentID
}

type SegmentStore interface {
	Put(Segment) error
	PutMany([]Segment) error

	Get(SegmentID) (Segment, error)
	Has(SegmentID) (bool, error)
	Delete(SegmentID) error
}
