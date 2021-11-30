package provider

import (
	"encoding/binary"
	"sync"

	"github.com/bits-and-blooms/bitset"
	"github.com/fxamacker/cbor/v2"
	"github.com/gogo/protobuf/proto"
	"github.com/zeebo/blake3"

	"github.com/memoio/go-mefs-v2/build"
	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12_381"
	"github.com/memoio/go-mefs-v2/lib/crypto/pdp"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

type SegMgr struct {
	sync.RWMutex

	ds store.KVStore

	localID uint64

	epoch uint64
	users []uint64
	sInfo map[uint64]*SegInfo // key: userID
}

type SegInfo struct {
	userID uint64
	fsID   []byte

	lastChal uint64

	bucket  uint64 // total bucket
	buckets []*BucketInfo
}

type BucketSet struct {
	Size       uint64
	AvalStripe *bitset.BitSet
	AccHw      bls.Fr
}

type BucketInfo struct {
	BucketSet
	Chunk   uint32
	StQueue types.StripeQueue
}

func NewSegMgr(localID uint64, ds store.KVStore) *SegMgr {
	s := &SegMgr{
		localID: localID,
		ds:      ds,
		users:   make([]uint64, 0, 128),
		sInfo:   make(map[uint64]*SegInfo),
	}

	s.load()

	return s
}

func (s *SegMgr) load() {
	key := store.NewKey(pb.MetaType_Chal_UsersKey)
	val, err := s.ds.Get(key)
	if err != nil {
		return
	}

	for i := 0; i < len(val)/8; i++ {
		pid := binary.BigEndian.Uint64(val[8*i : 8*(i+1)])
		s.loadFs(pid, false)
	}
}

func (s *SegMgr) save() error {
	buf := make([]byte, 8*len(s.users))
	for i, pid := range s.users {
		binary.BigEndian.PutUint64(buf[8*i:8*(i+1)], pid)
	}

	key := store.NewKey(pb.MetaType_Chal_UsersKey)
	return s.ds.Put(key, buf)
}

func (s *SegMgr) loadFs(userID uint64, save bool) *SegInfo {
	si, ok := s.sInfo[userID]
	if ok {
		key := store.NewKey(pb.MetaType_ST_PDPPublicKey, userID)
		data, err := s.ds.Get(key)
		if err != nil {
			return nil
		}

		pk, err := pdp.DeserializePublicKey(data)
		if err != nil {
			return nil
		}

		// load from local
		si = &SegInfo{
			userID:  userID,
			fsID:    pk.VerifyKey().Hash(),
			buckets: make([]*BucketInfo, 0, 2),
		}

		key = store.NewKey(pb.MetaType_Chal_ProofKey, userID)
		data, err = s.ds.Get(key)
		if err == nil && len(data) >= 8 {
			si.lastChal = binary.BigEndian.Uint64(data)
		}

		key = store.NewKey(pb.MetaType_ST_BucketOptKey, userID)
		data, err = s.ds.Get(key)
		if err == nil && len(data) >= 8 {
			si.bucket = binary.BigEndian.Uint64(data)
		}

		si.buckets = make([]*BucketInfo, si.bucket)

		for i := uint64(0); i < si.bucket; i++ {
			bm := &BucketInfo{
				BucketSet: BucketSet{
					AvalStripe: bitset.New(1024),
					AccHw:      bls.ZERO,
				},
			}

			key := store.NewKey(pb.MetaType_Chal_BucketInfoKey, userID, i)
			data, err = s.ds.Get(key)
			if err == nil {
				cbor.Unmarshal(data, bm)
			} else {
				pbo := new(pb.BucketOption)
				key := store.NewKey(pb.MetaType_ST_BucketOptKey, userID, i)
				data, err := s.ds.Get(key)
				if err != nil {
					return si
				}
				err = proto.Unmarshal(data, pbo)
				if err != nil {
					return si
				}
				bm.Chunk = pbo.DataCount + pbo.ParityCount
			}
			si.buckets[i] = bm
		}

		s.users = append(s.users, userID)
		s.sInfo[userID] = si

		if save {
			s.save()
		}
	}

	return si
}

func (s *SegMgr) loadBucket(si *SegInfo, bucketID uint64) *BucketInfo {
	if bucketID < si.bucket {
		return si.buckets[bucketID]
	}

	for i := si.bucket; i < bucketID; i++ {
		bm := &BucketInfo{
			BucketSet: BucketSet{
				AvalStripe: bitset.New(1024),
				AccHw:      bls.ZERO,
			},
		}

		key := store.NewKey(pb.MetaType_Chal_BucketInfoKey, si.userID, i)
		data, err := s.ds.Get(key)
		if err == nil {
			cbor.Unmarshal(data, bm)
		} else {
			pbo := new(pb.BucketOption)
			key := store.NewKey(pb.MetaType_ST_BucketOptKey, si.userID, i)
			data, err := s.ds.Get(key)
			if err != nil {
				return nil
			}
			err = proto.Unmarshal(data, pbo)
			if err != nil {
				return nil
			}
			bm.Chunk = pbo.DataCount + pbo.ParityCount
		}

		si.buckets = append(si.buckets, bm)
	}

	return si.buckets[bucketID]
}

func (s *SegMgr) AddStripe(userID, bucketID, stripeStart, stripeLength, proID uint64, chunkID uint32) {
	s.Lock()
	defer s.Unlock()

	if proID != s.localID {
		return
	}

	si := s.loadFs(userID, true)

	bm := s.loadBucket(si, bucketID)

	stripe := &types.Stripe{
		ChunkID: chunkID,
		Start:   stripeStart,
		Length:  stripeLength,
	}
	bm.StQueue.Push(stripe)

	var HWi bls.Fr
	for i := stripeStart; i < stripeStart+stripeLength; i++ {
		bm.Size += build.DefaultSegSize

		bm.AvalStripe.Set(uint(i))

		// calculate fr
		sid := segment.CreateSegmentID(si.fsID, bucketID, i, chunkID)
		h := blake3.Sum256(sid)
		bls.FrFromBytes(&HWi, h[:])
		bls.FrAddMod(&bm.AccHw, &bm.AccHw, &HWi)
	}

	// save
	key := store.NewKey(pb.MetaType_Chal_BucketInfoKey, userID, bucketID)
	data, err := cbor.Marshal(bm)
	if err != nil {
		logger.Debug("marshal fails:", err)
		return
	}

	s.ds.Put(key, data)
}

func (s *SegMgr) UpdateEpoch(epoch uint64) {
	// save epoch avail map; fr
	for _, userID := range s.users {
		si := s.loadFs(userID, false)
		for i := uint64(0); i < si.bucket; i++ {
			bm := s.loadBucket(si, i)

			key := store.NewKey(pb.MetaType_Chal_BucketInfoKey, userID, i, epoch)
			data, err := cbor.Marshal(bm.BucketSet)
			if err != nil {
				logger.Debug("marshal fails:", err)
				return
			}
			s.ds.Put(key, data)
		}
	}
	// prove
}

func (s *SegMgr) Challenge(userID uint64) {
	// save epoch avail map; fr
	// prove
	si := s.loadFs(userID, false)
	if si.lastChal < s.epoch {
		// challenge
		return
	}
}
