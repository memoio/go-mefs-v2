package state

import (
	"encoding/binary"

	"github.com/bits-and-blooms/bitset"
	"github.com/golang/protobuf/proto"
	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12_381"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/zeebo/blake3"
)

func (s *stateMgr) getBucketManage(spu *segPerUser, userID, bucketID uint64) *bucketManage {
	bm, ok := spu.buckets[bucketID]
	if ok {
		return bm
	}

	bm = &bucketManage{
		chunks:  make([]*chunkManage, 0),
		accHw:   make(map[uint64]*chalManage),
		unavail: bitset.New(1024),
	}

	spu.buckets[bucketID] = bm

	pbo := new(pb.BucketOption)
	key := store.NewKey(pb.MetaType_ST_BucketOptKey, userID, bucketID)
	data, err := s.ds.Get(key)
	if err != nil {
		return bm
	}

	err = proto.Unmarshal(data, pbo)
	if err != nil {
		return bm
	}
	bm.chunks = make([]*chunkManage, pbo.DataCount+pbo.ParityCount)

	return bm
}

func (s *stateMgr) getChalManage(bm *bucketManage, userID, bucketID, proID uint64) *chalManage {
	cm, ok := bm.accHw[proID]
	if ok {
		return cm
	}

	cm = &chalManage{
		size:  0,
		accFr: bls.ZERO,
		avail: bitset.New(1024),
	}

	bm.accHw[proID] = cm

	key := store.NewKey(pb.MetaType_St_SegAggKey, userID, bucketID, proID)
	data, err := s.ds.Get(key)
	if err != nil {
		return cm
	}

	err = cm.Deserialize(data)
	if err != nil {
		return cm
	}
	return cm
}

func (s *stateMgr) AddBucket(userID, bucketID uint64, pbo *pb.BucketOption) error {
	s.Lock()
	defer s.Unlock()

	uinfo, err := s.getUser(userID)
	if err != nil {
		return err
	}

	if uinfo.nextBucket != bucketID {
		return ErrRes
	}

	bm := &bucketManage{
		chunks:  make([]*chunkManage, pbo.DataCount+pbo.ParityCount),
		accHw:   make(map[uint64]*chalManage),
		unavail: bitset.New(1024),
	}

	uinfo.buckets[bucketID] = bm
	uinfo.nextBucket++

	// save
	key := store.NewKey(pb.MetaType_ST_BucketOptKey, userID, bucketID)
	data, err := proto.Marshal(pbo)
	if err != nil {
		return err
	}
	err = s.ds.Put(key, data)
	if err != nil {
		return err
	}

	key = store.NewKey(pb.MetaType_ST_BucketOptKey, userID)
	val := make([]byte, 8)
	binary.BigEndian.PutUint64(val, uinfo.nextBucket)
	err = s.ds.Put(key, val)
	if err != nil {
		return err
	}

	return nil
}

func (s *stateMgr) AddChunk(userID, bucketID, stripeStart, stripeLength, proID, nonce uint64, chunkID uint32) error {
	uinfo, err := s.getUser(userID)
	if err != nil {
		return err
	}

	if uinfo.nextBucket <= bucketID {
		return ErrRes
	}

	binfo := s.getBucketManage(uinfo, userID, bucketID)

	if int(chunkID) >= len(binfo.chunks) {
		return ErrRes
	}

	cm := s.getChalManage(binfo, userID, bucketID, proID)

	// check whether has it already
	for i := stripeStart; i < stripeStart+stripeLength; i++ {
		if cm.avail.Test(uint(i)) {
			return ErrRes
		}
	}

	cinfo := binfo.chunks[chunkID]
	if cinfo.stripe != nil && cinfo.stripe.ProID == proID && cinfo.stripe.Nonce == nonce && cinfo.stripe.Start+cinfo.stripe.Length == stripeStart {
		cinfo.stripe.Length += stripeLength
	} else {
		cinfo.stripe = &types.AggStripe{
			Nonce:  nonce,
			ProID:  proID,
			Start:  stripeStart,
			Length: stripeLength,
		}
	}

	var HWi bls.Fr
	for i := stripeStart; i < stripeStart+stripeLength; i++ {
		cm.avail.Set(uint(i))

		// calculate fr
		sid := segment.CreateSegmentID(uinfo.fsID, bucketID, i, chunkID)
		h := blake3.Sum256(sid)
		bls.FrFromBytes(&HWi, h[:])
		bls.FrAddMod(&cm.accFr, &cm.accFr, &HWi)
	}

	// save
	key := store.NewKey(pb.MetaType_St_SegAggKey, userID, bucketID, proID)
	data, err := cm.Serialize()
	if err != nil {
		return err
	}
	err = s.ds.Put(key, data)
	if err != nil {
		return err
	}

	return nil
}
