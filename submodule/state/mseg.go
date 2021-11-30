package state

import (
	"encoding/binary"

	"github.com/bits-and-blooms/bitset"
	"github.com/gogo/protobuf/proto"
	"github.com/zeebo/blake3"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/build"
	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12_381"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

// key: pb.MetaType_ST_BucketOptKey/userID; val: next bucketID
// key: pb.MetaType_ST_BucketOptKey/userID/bucketID; val: bucket ops
// key: pb.MetaType_ST_SegLocKey/userID/bucketID/chunkID; val: stripeID of largest stripe
// key: pb.MetaType_ST_SegLocKey/userID/bucketID/chunkID/stripeID; val: stripe
// key: pb.MetaType_St_SegMapKey/userID/bucketID/proID; val: bitmap, accFr

// for chal
// key: pb.MetaType_St_SegMapKey/userID/bucketID/proID/epoch; val: bitmap, accFr

func (s *StateMgr) getBucketManage(spu *segPerUser, userID, bucketID uint64) *bucketManage {
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

func (s *StateMgr) getChunkManage(bm *bucketManage, userID, bucketID uint64, chunkID uint32) *chunkManage {
	cm := bm.chunks[chunkID]

	if cm == nil {
		cm = &chunkManage{}
		bm.chunks[chunkID] = cm
	}

	key := store.NewKey(pb.MetaType_ST_SegLocKey, userID, bucketID, chunkID)
	data, err := s.ds.Get(key)
	if err != nil {
		return cm
	}

	if len(data) >= 8 {
		stripeID := binary.BigEndian.Uint64(data)
		key := store.NewKey(pb.MetaType_ST_SegLocKey, userID, bucketID, chunkID, stripeID)
		data, err := s.ds.Get(key)
		if err != nil {
			return cm
		}
		as := new(types.AggStripe)
		err = as.Deserialize(data)
		if err == nil {
			cm.stripe = as
		}
	}

	return cm
}

func (s *StateMgr) getChalManage(bm *bucketManage, userID, bucketID, proID uint64) *chalManage {
	cm, ok := bm.accHw[proID]
	if ok {
		return cm
	}

	cm = &chalManage{
		size:      0,
		accFr:     bls.ZERO,
		deletedFr: bls.ZERO,
		avail:     bitset.New(1024),
	}

	bm.accHw[proID] = cm

	key := store.NewKey(pb.MetaType_St_SegMapKey, userID, bucketID, proID)
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

func (s *StateMgr) AddBucket(msg *tx.Message) (types.MsgID, error) {
	tbp := new(tx.BucketParams)
	err := tbp.Deserialize(msg.Params)
	if err != nil {
		return s.root, err
	}

	s.Lock()
	defer s.Unlock()

	uinfo, ok := s.sInfo[msg.From]
	if !ok {
		uinfo, err = s.loadUser(msg.From)
		if err != nil {
			return s.root, err
		}
		s.sInfo[msg.From] = uinfo
	}

	if uinfo.nextBucket != tbp.BucketID {
		return s.root, xerrors.Errorf("add bucket %d err: %w", tbp.BucketID, ErrBucket)
	}

	bm := &bucketManage{
		chunks:  make([]*chunkManage, tbp.DataCount+tbp.ParityCount),
		accHw:   make(map[uint64]*chalManage),
		unavail: bitset.New(1024),
	}

	uinfo.buckets[tbp.BucketID] = bm
	uinfo.nextBucket++

	// save
	key := store.NewKey(pb.MetaType_ST_BucketOptKey, msg.From, tbp.BucketID)
	data, err := proto.Marshal(&tbp.BucketOption)
	if err != nil {
		return s.root, err
	}
	err = s.ds.Put(key, data)
	if err != nil {
		return s.root, err
	}

	key = store.NewKey(pb.MetaType_ST_BucketOptKey, msg.From)
	val := make([]byte, 8)
	binary.BigEndian.PutUint64(val, uinfo.nextBucket)
	err = s.ds.Put(key, val)
	if err != nil {
		return s.root, err
	}

	s.newRoot(msg.Params)

	return s.root, nil
}

func (s *StateMgr) AddChunk(userID, bucketID, stripeStart, stripeLength, proID, nonce uint64, chunkID uint32) error {
	var err error
	uinfo, ok := s.sInfo[userID]
	if !ok {
		uinfo, err = s.loadUser(userID)
		if err != nil {
			return err
		}
		s.sInfo[userID] = uinfo
	}
	if uinfo.nextBucket <= bucketID {
		return xerrors.Errorf("add chunk %d err: %w", bucketID, ErrBucket)
	}

	binfo := s.getBucketManage(uinfo, userID, bucketID)

	if int(chunkID) >= len(binfo.chunks) {
		return xerrors.Errorf("add chunk %d err: %w", chunkID, ErrChunk)
	}

	cm := s.getChalManage(binfo, userID, bucketID, proID)

	// check whether has it already
	for i := stripeStart; i < stripeStart+stripeLength; i++ {
		if cm.avail.Test(uint(i)) {
			return xerrors.Errorf("add chunk %d_%d_%d err:%w, ", bucketID, i, chunkID, ErrDuplicate)
		}
	}

	cinfo := s.getChunkManage(binfo, userID, bucketID, chunkID)
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
		cm.size += build.DefaultSegSize

		cm.avail.Set(uint(i))

		// calculate fr
		sid := segment.CreateSegmentID(uinfo.fsID, bucketID, i, chunkID)
		h := blake3.Sum256(sid)
		bls.FrFromBytes(&HWi, h[:])
		bls.FrAddMod(&cm.accFr, &cm.accFr, &HWi)
	}

	// save
	key := store.NewKey(pb.MetaType_St_SegMapKey, userID, bucketID, proID)
	data, err := cm.Serialize()
	if err != nil {
		return err
	}
	err = s.ds.Put(key, data)
	if err != nil {
		return err
	}

	key = store.NewKey(pb.MetaType_St_SegMapKey, userID, bucketID, proID, s.epochInfo.Epoch)
	err = s.ds.Put(key, data)
	if err != nil {
		return err
	}

	key = store.NewKey(pb.MetaType_ST_SegLocKey, userID, bucketID, chunkID, stripeStart)
	data, err = cinfo.stripe.Serialize()
	if err != nil {
		return err
	}
	err = s.ds.Put(key, data)
	if err != nil {
		return err
	}

	key = store.NewKey(pb.MetaType_ST_SegLocKey, userID, bucketID, chunkID)
	val := make([]byte, 8)
	binary.BigEndian.PutUint64(val, stripeStart)
	err = s.ds.Put(key, val)
	if err != nil {
		return err
	}

	return nil
}

func (s *StateMgr) CanAddBucket(msg *tx.Message) (types.MsgID, error) {
	tbp := new(tx.BucketParams)
	err := tbp.Deserialize(msg.Params)
	if err != nil {
		return s.validateRoot, err
	}

	s.Lock()
	defer s.Unlock()

	uinfo, ok := s.validateSInfo[msg.From]
	if !ok {
		uinfo, err = s.loadUser(msg.From)
		if err != nil {
			return s.validateRoot, err
		}
		s.validateSInfo[msg.From] = uinfo
	}

	if uinfo.nextBucket != tbp.BucketID {
		return s.validateRoot, xerrors.Errorf("add bucket %d err:%w, ", tbp.BucketID, ErrBucket)
	}

	bm := &bucketManage{
		chunks:  make([]*chunkManage, tbp.DataCount+tbp.ParityCount),
		accHw:   make(map[uint64]*chalManage),
		unavail: bitset.New(1024),
	}

	uinfo.buckets[tbp.BucketID] = bm
	uinfo.nextBucket++

	s.newValidateRoot(msg.Params)

	return s.validateRoot, nil
}

func (s *StateMgr) CanAddChunk(userID, bucketID, stripeStart, stripeLength, proID, nonce uint64, chunkID uint32) error {
	var err error
	uinfo, ok := s.validateSInfo[userID]
	if !ok {
		uinfo, err = s.loadUser(userID)
		if err != nil {
			return err
		}
		s.validateSInfo[userID] = uinfo
	}
	if uinfo.nextBucket <= bucketID {
		return xerrors.Errorf("add chunk %d %w", bucketID, ErrBucket)
	}

	binfo := s.getBucketManage(uinfo, userID, bucketID)

	if int(chunkID) >= len(binfo.chunks) {
		return xerrors.Errorf("add chunk %d %w", chunkID, ErrChunk)
	}

	cm := s.getChalManage(binfo, userID, bucketID, proID)

	// check whether has it already
	for i := stripeStart; i < stripeStart+stripeLength; i++ {
		if cm.avail.Test(uint(i)) {
			return xerrors.Errorf("add chunk %d_%d_%d %w", bucketID, i, chunkID, ErrDuplicate)
		}
	}

	cinfo := s.getChunkManage(binfo, userID, bucketID, chunkID)
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
		cm.size += build.DefaultSegSize
		cm.avail.Set(uint(i))

		// calculate fr
		sid := segment.CreateSegmentID(uinfo.fsID, bucketID, i, chunkID)
		h := blake3.Sum256(sid)
		bls.FrFromBytes(&HWi, h[:])
		bls.FrAddMod(&cm.accFr, &cm.accFr, &HWi)
	}

	return nil
}
