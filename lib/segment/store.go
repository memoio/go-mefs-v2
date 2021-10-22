package segment

import (
	lru "github.com/hashicorp/golang-lru"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/mr-tron/base58/base58"
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
	key := seg.SegmentID().Bytes()
	skey := []byte(base58.Encode(key[:20]) + "/" + base58.Encode(key[20:]))

	return ss.FileStore.Put(skey, seg.RawData())
}

func (ss segStore) PutMany(segs []Segment) error {
	for _, seg := range segs {
		key := seg.SegmentID().Bytes()
		skey := []byte(base58.Encode(key[:20]) + "/" + base58.Encode(key[20:]))
		err := ss.FileStore.Put(skey, seg.RawData())
		if err != nil {
			return err
		}
	}
	return nil
}

func (ss segStore) Get(segID SegmentID) (Segment, error) {

	key := segID.Bytes()
	skey := []byte(base58.Encode(key[:20]) + "/" + base58.Encode(key[20:]))

	data, err := ss.FileStore.Get(skey)
	if err != nil {
		return nil, err
	}

	bs := NewBaseSegment(data, segID)

	return bs, nil
}

func (ss segStore) Has(segID SegmentID) (bool, error) {
	key := segID.Bytes()
	skey := []byte(base58.Encode(key[:20]) + "/" + base58.Encode(key[20:]))

	return ss.FileStore.Has(skey)
}

func (ss segStore) Delete(segID SegmentID) error {
	key := segID.Bytes()
	skey := []byte(base58.Encode(key[:20]) + "/" + base58.Encode(key[20:]))

	return ss.FileStore.Delete(skey)
}
