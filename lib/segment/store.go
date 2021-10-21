package segment

import (
	lru "github.com/hashicorp/golang-lru"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

var _ SegmentStore = (*segStore)(nil)

type segStore struct {
	store.FileStore

	// todo, add cache ops
	cache *lru.ARCCache
}

func NewSegStore(fs store.FileStore) (SegmentStore, error) {
	cache, err := lru.NewARC(1024)
	if err != nil {
		return nil, err
	}

	return &segStore{fs, cache}, nil
}

func (ss segStore) Put(seg Segment) error {
	return ss.FileStore.Put(seg.SegmentID().Bytes(), seg.RawData())
}

func (ss segStore) PutMany(segs []Segment) error {
	for _, seg := range segs {
		err := ss.FileStore.Put(seg.SegmentID().Bytes(), seg.RawData())
		if err != nil {
			return err
		}
	}
	return nil
}

func (ss segStore) Get(segID SegmentID) (Segment, error) {
	bs := new(BaseSegment)

	data, err := ss.FileStore.Get(segID.Bytes())
	if err != nil {
		return bs, err
	}

	bs.SetID(segID)
	bs.SetData(data)

	return bs, nil
}

func (ss segStore) Has(segID SegmentID) (bool, error) {
	return ss.FileStore.Has(segID.Bytes())
}

func (ss segStore) Delete(segID SegmentID) error {
	return ss.FileStore.Delete(segID.Bytes())
}
