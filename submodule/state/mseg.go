package state

import (
	"encoding/binary"

	"github.com/bits-and-blooms/bitset"
	"github.com/gogo/protobuf/proto"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

// for repair
// key: pb.MetaType_ST_BucketOptKey/userID; val: next bucketID
// key: pb.MetaType_ST_BucketOptKey/userID/bucketID; val: bucket ops
// key: pb.MetaType_ST_SegLocKey/userID/bucketID/chunkID; val: stripeID of largest stripe
// key: pb.MetaType_ST_SegLocKey/userID/bucketID/chunkID/stripeID; val: aggstripe
// key: pb.MetaType_St_SegMapKey/userID/bucketID/proID; val: bitmap

func (s *StateMgr) getBucketManage(spu *segPerUser, bucketID uint64) *bucketManage {
	bm, ok := spu.buckets[bucketID]
	if ok {
		return bm
	}

	bm = &bucketManage{
		chunks:  make([]*types.AggStripe, 0),
		stripes: make(map[uint64]*bitset.BitSet),
	}

	spu.buckets[bucketID] = bm

	pbo := new(pb.BucketOption)
	key := store.NewKey(pb.MetaType_ST_BucketOptKey, spu.userID, bucketID)
	data, err := s.ds.Get(key)
	if err != nil {
		return bm
	}

	err = proto.Unmarshal(data, pbo)
	if err != nil {
		return bm
	}
	bm.chunks = make([]*types.AggStripe, pbo.DataCount+pbo.ParityCount)
	for i := uint32(0); i < pbo.DataCount+pbo.ParityCount; i++ {
		key := store.NewKey(pb.MetaType_ST_SegLocKey, spu.userID, bucketID, i)
		data, err := s.ds.Get(key)
		if err != nil || len(data) < 8 {
			continue
		}

		stripeID := binary.BigEndian.Uint64(data)
		key = store.NewKey(pb.MetaType_ST_SegLocKey, spu.userID, bucketID, i, stripeID)
		data, err = s.ds.Get(key)
		if err != nil {
			continue
		}
		as := new(types.AggStripe)
		err = as.Deserialize(data)
		if err == nil {
			bm.chunks[i] = as
		}
	}

	return bm
}

func (s *StateMgr) getStripeBitMap(bm *bucketManage, userID, bucketID, proID uint64) *bitset.BitSet {
	cm, ok := bm.stripes[proID]
	if ok {
		return cm
	}

	cm = bitset.New(1024)

	key := store.NewKey(pb.MetaType_ST_SegMapKey, userID, bucketID, proID)
	data, err := s.ds.Get(key)
	if err != nil {
		bm.stripes[proID] = cm
		return cm
	}

	bs := new(bitsetStored)
	err = bs.Deserialize(data)
	if err != nil {
		bm.stripes[proID] = cm
		return cm
	}

	ncm := bitset.From(bs.Val)
	bm.stripes[proID] = ncm

	return ncm
}

func (s *StateMgr) addBucket(msg *tx.Message, tds store.TxnStore) error {
	tbp := new(tx.BucketParams)
	err := tbp.Deserialize(msg.Params)
	if err != nil {
		return err
	}

	uinfo, ok := s.sInfo[msg.From]
	if !ok {
		uinfo, err = s.loadUser(msg.From)
		if err != nil {
			return err
		}
		s.sInfo[msg.From] = uinfo
	}

	if uinfo.nextBucket != tbp.BucketID {
		return xerrors.Errorf("add bucket err, expected %d, got %d", uinfo.nextBucket, tbp.BucketID)
	}

	bm := &bucketManage{
		chunks:  make([]*types.AggStripe, tbp.DataCount+tbp.ParityCount),
		stripes: make(map[uint64]*bitset.BitSet),
	}

	uinfo.buckets[tbp.BucketID] = bm
	uinfo.nextBucket++

	// save
	key := store.NewKey(pb.MetaType_ST_BucketOptKey, msg.From, tbp.BucketID)
	data, err := proto.Marshal(&tbp.BucketOption)
	if err != nil {
		return err
	}
	err = tds.Put(key, data)
	if err != nil {
		return err
	}

	key = store.NewKey(pb.MetaType_ST_BucketOptKey, msg.From)
	val := make([]byte, 8)
	binary.BigEndian.PutUint64(val, uinfo.nextBucket)
	err = tds.Put(key, val)
	if err != nil {
		return err
	}

	return nil
}

func (s *StateMgr) canAddBucket(msg *tx.Message) error {
	tbp := new(tx.BucketParams)
	err := tbp.Deserialize(msg.Params)
	if err != nil {
		return err
	}

	uinfo, ok := s.validateSInfo[msg.From]
	if !ok {
		uinfo, err = s.loadUser(msg.From)
		if err != nil {
			return err
		}
		s.validateSInfo[msg.From] = uinfo
	}

	if uinfo.nextBucket != tbp.BucketID {
		return xerrors.Errorf("add bucket err, expected %d, got %d", uinfo.nextBucket, tbp.BucketID)
	}

	bm := &bucketManage{
		chunks:  make([]*types.AggStripe, tbp.DataCount+tbp.ParityCount),
		stripes: make(map[uint64]*bitset.BitSet),
	}

	uinfo.buckets[tbp.BucketID] = bm
	uinfo.nextBucket++

	return nil
}

func (s *StateMgr) addChunk(userID, bucketID, stripeStart, stripeLength, proID, nonce uint64, chunkID uint32, tds store.TxnStore) error {
	uinfo, ok := s.sInfo[userID]
	if !ok {
		var err error
		uinfo, err = s.loadUser(userID)
		if err != nil {
			return err
		}
		s.sInfo[userID] = uinfo
	}
	if uinfo.nextBucket <= bucketID {
		return xerrors.Errorf("add chunk bucket too large, expected %d, got %d", uinfo.nextBucket, bucketID)
	}

	binfo := s.getBucketManage(uinfo, bucketID)

	if int(chunkID) >= len(binfo.chunks) {
		return xerrors.Errorf("add chunk too large,  expected less %d, got %d", len(binfo.chunks), chunkID)
	}

	cm := s.getStripeBitMap(binfo, userID, bucketID, proID)
	// check whether has it already
	for i := stripeStart; i < stripeStart+stripeLength; i++ {
		if cm.Test(uint(i)) {
			return xerrors.Errorf("add duplicated chunk %d_%d_%d", bucketID, i, chunkID)
		}
	}

	cinfo := binfo.chunks[chunkID]
	if cinfo != nil && cinfo.ProID == proID && cinfo.Order == nonce && cinfo.Start+cinfo.Length == stripeStart {
		cinfo.Length += stripeLength
	} else {
		cinfo = &types.AggStripe{
			Order:  nonce,
			ProID:  proID,
			Start:  stripeStart,
			Length: stripeLength,
		}
		binfo.chunks[chunkID] = cinfo
	}

	for i := stripeStart; i < stripeStart+stripeLength; i++ {
		cm.Set(uint(i))
	}

	// save
	key := store.NewKey(pb.MetaType_ST_SegMapKey, userID, bucketID, proID)
	bs := &bitsetStored{
		Val: cm.Bytes(),
	}
	data, err := bs.Serialize()
	if err != nil {
		return err
	}
	err = tds.Put(key, data)
	if err != nil {
		return err
	}

	key = store.NewKey(pb.MetaType_ST_SegMapKey, userID, bucketID, proID, s.ceInfo.epoch)
	err = tds.Put(key, data)
	if err != nil {
		return err
	}

	key = store.NewKey(pb.MetaType_ST_SegLocKey, userID, bucketID, chunkID, stripeStart)
	data, err = cinfo.Serialize()
	if err != nil {
		return err
	}
	err = tds.Put(key, data)
	if err != nil {
		return err
	}

	key = store.NewKey(pb.MetaType_ST_SegLocKey, userID, bucketID, chunkID)
	val := make([]byte, 8)
	binary.BigEndian.PutUint64(val, stripeStart)
	err = tds.Put(key, val)
	if err != nil {
		return err
	}

	return nil
}

func (s *StateMgr) canAddChunk(userID, bucketID, stripeStart, stripeLength, proID, nonce uint64, chunkID uint32) error {
	uinfo, ok := s.validateSInfo[userID]
	if !ok {
		var err error
		uinfo, err = s.loadUser(userID)
		if err != nil {
			return err
		}
		s.validateSInfo[userID] = uinfo
	}

	if uinfo.nextBucket <= bucketID {
		return xerrors.Errorf("add chunk bucket too large, expected %d, got %d", uinfo.nextBucket, bucketID)
	}

	binfo := s.getBucketManage(uinfo, bucketID)
	if int(chunkID) >= len(binfo.chunks) {
		return xerrors.Errorf("add chunk too large,  expected less %d, got %d", len(binfo.chunks), chunkID)
	}

	cm := s.getStripeBitMap(binfo, userID, bucketID, proID)
	// check whether has it already
	for i := stripeStart; i < stripeStart+stripeLength; i++ {
		if cm.Test(uint(i)) {
			return xerrors.Errorf("add duplicated chunk %d_%d_%d", bucketID, i, chunkID)
		}
	}

	cinfo := binfo.chunks[chunkID]
	if cinfo != nil && cinfo.ProID == proID && cinfo.Order == nonce && cinfo.Start+cinfo.Length == stripeStart {
		cinfo.Length += stripeLength
	} else {
		cinfo = &types.AggStripe{
			Order:  nonce,
			ProID:  proID,
			Start:  stripeStart,
			Length: stripeLength,
		}
		binfo.chunks[chunkID] = cinfo
	}

	for i := stripeStart; i < stripeStart+stripeLength; i++ {
		cm.Set(uint(i))
	}
	return nil
}
