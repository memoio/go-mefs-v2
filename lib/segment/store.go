package segment

import "github.com/memoio/go-mefs-v2/lib/types"

var _ SegmentStore = (*segStore)(nil)

type segStore struct {
	types.FileStore
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
