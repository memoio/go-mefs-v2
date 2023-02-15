package state

import (
	"encoding/binary"

	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"golang.org/x/xerrors"
)

// bucket
// key: pb.MetaType_ST_BucMetaKey/userID/bucketID; val: name

// object
// key: pb.MetaType_ST_ObjMetaKey/userID/bucketID/objectID; val: objMeta
// key: pb.MetaType_ST_ObjMetaKey/etag{/count}; value: userID/bucketID/objectID
// key: pb.MetaType_ST_ObjMetaKey/etag/0; value: count

func (s *StateMgr) addBucMeta(msg *tx.Message) error {
	bmp := new(tx.BucMetaParas)
	err := bmp.Deserialize(msg.Params)
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

	if uinfo.nextBucket <= bmp.BucketID {
		return xerrors.Errorf("add bucket meta err, expected less than %d, got %d", uinfo.nextBucket, bmp.BucketID)
	}

	key := store.NewKey(pb.MetaType_ST_BucMetaKey, msg.From, bmp.BucketID)
	return s.put(key, msg.Params)
}

func (s *StateMgr) canAddBucMeta(msg *tx.Message) error {
	bmp := new(tx.BucMetaParas)
	err := bmp.Deserialize(msg.Params)
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

	if uinfo.nextBucket <= bmp.BucketID {
		return xerrors.Errorf("add bucket meta err, expected less than %d, got %d", uinfo.nextBucket, bmp.BucketID)
	}

	return nil
}

func (s *StateMgr) addObjMeta(msg *tx.Message) error {
	omp := new(tx.ObjMetaParas)
	err := omp.Deserialize(msg.Params)
	if err != nil {
		return err
	}

	da, err := omp.ObjMetaValue.Serialize()
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

	if uinfo.nextBucket <= omp.BucketID {
		return xerrors.Errorf("add object meta err, expected less than %d, got %d", uinfo.nextBucket, omp.BucketID)
	}

	cnt := uint64(0)
	key := store.NewKey(pb.MetaType_ST_ObjMetaKey, omp.ETag, 0)
	val, err := s.get(key)
	if err == nil && len(val) >= 8 {
		cnt = binary.BigEndian.Uint64(val[:8])
		val = make([]byte, 8)
		binary.BigEndian.PutUint64(val[:8], cnt+1)
		err = s.put(key, val)
		if err != nil {
			return err
		}
	}

	if cnt > 0 {
		key = store.NewKey(pb.MetaType_ST_ObjMetaKey, omp.ETag, cnt)
	} else {
		key = store.NewKey(pb.MetaType_ST_ObjMetaKey, omp.ETag)
	}

	omk := new(tx.ObjMetaKey)
	omk.UserID = msg.From
	omk.BucketID = omp.BucketID
	omk.ObjectID = omp.ObjectID
	val, err = omk.Serialize()
	if err != nil {
		return err
	}

	err = s.put(key, val)
	if err != nil {
		return err
	}

	key = store.NewKey(pb.MetaType_ST_ObjMetaKey, msg.From, omp.BucketID, omp.ObjectID)
	return s.put(key, da)
}

func (s *StateMgr) canAddObjMeta(msg *tx.Message) error {
	omp := new(tx.ObjMetaParas)
	err := omp.Deserialize(msg.Params)
	if err != nil {
		return err
	}

	_, err = omp.ObjMetaValue.Serialize()
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

	if uinfo.nextBucket <= omp.BucketID {
		return xerrors.Errorf("add object meta err, expected less than %d, got %d", uinfo.nextBucket, omp.BucketID)
	}

	return nil
}
