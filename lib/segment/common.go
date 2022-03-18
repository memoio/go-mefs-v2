package segment

type SegmentID interface {
	GetFsID() []byte
	GetBucketID() uint64
	GetStripeID() uint64
	GetChunkID() uint32

	SetBucketID(sID uint64)
	SetStripeID(sID uint64)
	SetChunkID(cID uint32)

	Bytes() []byte
	ToString() string
	String() string

	ShortBytes() []byte
	ShortString() string
}

type Segment interface {
	SetID(SegmentID)
	SetData([]byte)
	SegmentID() SegmentID
	Data() []byte
	Content() ([]byte, error)
	Tags() ([][]byte, error)
	Serialize() ([]byte, error)
	Deserialize([]byte) error

	IsValid(size int) error
}

type SegmentStore interface {
	Put(Segment) error
	PutMany([]Segment) error

	Get(SegmentID) (Segment, error)
	Has(SegmentID) (bool, error)
	Delete(SegmentID) error
}
