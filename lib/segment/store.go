package segment

import (
	"strconv"

	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/mr-tron/base58/base58"
)

var _ SegmentStore = (*segStore)(nil)

type segStore struct {
	store.FileStore
}

func NewSegStore(fs store.FileStore) (SegmentStore, error) {
	return &segStore{fs}, nil
}

func (ss segStore) Put(seg Segment) error {
	skey := convertKey(seg.SegmentID())

	return ss.FileStore.Put(skey, seg.Data())
}

func (ss segStore) PutMany(segs []Segment) error {
	for _, seg := range segs {
		skey := convertKey(seg.SegmentID())
		err := ss.FileStore.Put(skey, seg.Data())
		if err != nil {
			return err
		}
	}
	return nil
}

func (ss segStore) Get(segID SegmentID) (Segment, error) {
	skey := convertKey(segID)

	data, err := ss.FileStore.Get(skey)
	if err != nil {
		return nil, err
	}

	bs := NewBaseSegment(data, segID)

	return bs, nil
}

func (ss segStore) Has(segID SegmentID) (bool, error) {
	skey := convertKey(segID)

	return ss.FileStore.Has(skey)
}

func (ss segStore) Delete(segID SegmentID) error {
	skey := convertKey(segID)

	return ss.FileStore.Delete(skey)
}

func convertKey(segID SegmentID) []byte {
	sid := segID.GetStripeID() % 1024
	return []byte(base58.Encode(segID.GetFsID()) + "/" + strconv.FormatUint(sid, 10) + "/" + strconv.FormatUint(segID.GetBucketID(), 10) + "_" + strconv.FormatUint(segID.GetStripeID(), 10) + "_" + strconv.FormatUint(uint64(segID.GetChunkID()), 10))
}
