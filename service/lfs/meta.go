package lfs

import (
	"crypto/md5"
	"crypto/sha256"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
	rbtree "github.com/memoio/go-mefs-v2/lib/RbTree"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	mt "gitlab.com/NebulousLabs/merkletree"
)

type superBlock struct {
	sync.RWMutex
	pb.SuperBlockInfo
	dirty          bool
	write          bool              // set true when finish unfinished jobs
	ready          bool              // set true after loaded from local store
	bucketMax      uint64            // Get from chain; in case create too mant buckets
	bucketNameToID map[string]uint64 // bucketName -> bucketID
	buckets        []*bucket         // 所有的bucket信息
}

func NewSuperBlock() *superBlock {
	return &superBlock{
		SuperBlockInfo: pb.SuperBlockInfo{
			Version:      0,
			ReplicaNum:   3,
			NextBucketID: 0,
		},
		dirty:          true,
		bucketMax:      0,
		bucketNameToID: make(map[string]uint64),
	}
}

func (sbl *superBlock) Load(fsID []byte, ds store.KVStore) error {
	// from local
	key := store.NewKey(pb.MetaType_LFS_SuperBlockInfoKey, fsID)
	data, err := ds.Get(key)
	if err != nil {
		// if miss; init?
		return err
	}

	sbi := new(pb.SuperBlockInfo)
	err = proto.Unmarshal(data, sbi)
	if err != nil {
		return err
	}

	sbl.SuperBlockInfo = *sbi
	sbl.dirty = false
	sbl.buckets = make([]*bucket, sbl.NextBucketID)

	return nil
}

func (sbl *superBlock) Save(fsID []byte, ds store.KVStore) error {
	// to local
	if sbl.dirty {
		key := store.NewKey(pb.MetaType_LFS_SuperBlockInfoKey, fsID)
		data, err := proto.Marshal(&sbl.SuperBlockInfo)
		if err != nil {
			return err
		}
		err = ds.Put(key, data)
		if err != nil {
			return err
		}
		sbl.dirty = false
		return nil
	}

	return nil
}

type bucket struct {
	sync.RWMutex
	types.BucketInfo

	dirty   bool
	objects *rbtree.Tree // store objects; key is object name
	mtree   *mt.Tree     // update when done
}

func newBucket(bucketID uint64, bucketName string, opt *pb.BucketOption) *bucket {
	bi := types.BucketInfo{
		BucketOption: *opt,
		BucketInfo: pb.BucketInfo{
			BucketID: bucketID,
			CTime:    time.Now().Unix(),
			MTime:    time.Now().Unix(),
			Name:     bucketName,
		},
	}
	return &bucket{
		BucketInfo: bi,
		dirty:      true,
		objects:    rbtree.NewTree(),
		mtree:      mt.New(sha256.New()),
	}
}

func (bu *bucket) Load(fsID []byte, bucketID uint64, ds store.KVStore) error {
	// from local
	key := store.NewKey(pb.MetaType_LFS_BucketOptionKey, fsID, bucketID)
	data, err := ds.Get(key)
	if err != nil {
		// if miss; init?
		return err
	}

	bo := new(pb.BucketOption)
	err = proto.Unmarshal(data, bo)
	if err != nil {
		return err
	}

	key = store.NewKey(pb.MetaType_LFS_BucketInfoKey, fsID, bucketID)
	data, err = ds.Get(key)
	if err != nil {
		// if miss; init?
		return err
	}

	pbi := new(pb.BucketInfo)
	err = proto.Unmarshal(data, pbi)
	if err != nil {
		return err
	}

	bi := types.BucketInfo{
		BucketInfo:   *pbi,
		BucketOption: *bo,
	}

	bu.BucketInfo = bi

	return nil
}

func (bu *bucket) Save(fsID []byte, ds store.KVStore) error {
	// to local
	if bu.dirty {
		key := store.NewKey(pb.MetaType_LFS_BucketInfoKey, fsID, bu.BucketID)
		data, err := proto.Marshal(&bu.BucketInfo.BucketInfo)
		if err != nil {
			return err
		}
		err = ds.Put(key, data)
		if err != nil {
			return err
		}
		bu.dirty = false
	}

	return nil
}

func (bu *bucket) SaveOptions(fsID []byte, ds store.KVStore) error {
	key := store.NewKey(pb.MetaType_LFS_BucketOptionKey, fsID, bu.BucketID)
	data, err := proto.Marshal(&bu.BucketOption)
	if err != nil {
		return err
	}
	err = ds.Put(key, data)
	if err != nil {
		return err
	}

	return nil
}

type object struct {
	sync.RWMutex

	types.ObjectInfo

	ops      []uint64
	deletion bool
	dirty    bool
}

func NewObject() *object {
	return &object{
		ObjectInfo: types.ObjectInfo{
			Parts: make([]*pb.ObjectPartInfo, 0, 1),
		},
	}
}

func (ob *object) Load(fsID []byte, bucketID, objectID uint64, ds store.KVStore) error {
	// from local
	key := store.NewKey(pb.MetaType_LFS_ObjectInfoKey, fsID, bucketID, objectID)
	data, err := ds.Get(key)
	if err != nil {
		// if miss; init?
		return err
	}

	of := new(pb.ObjectForm)
	err = proto.Unmarshal(data, of)
	if err != nil {
		return err
	}

	for _, opID := range of.GetOpRecord() {
		or, err := loadOpRecord(fsID, bucketID, opID, ds)
		if err != nil {
			return err
		}

		switch or.GetType() {
		case pb.OpRecord_CreateObject:
			oi := new(pb.ObjectInfo)
			err = proto.Unmarshal(data, or)
			if err != nil {
				continue
			}
			ob.ObjectInfo.ObjectInfo = *oi

		case pb.OpRecord_AddData:
			pi := new(pb.ObjectPartInfo)
			err = proto.Unmarshal(data, pi)
			if err != nil {
				continue
			}

			if pi.ObjectID != ob.ObjectID {
				continue
			}

			ob.Length += pi.Length

			ob.ObjectInfo.Parts = append(ob.ObjectInfo.Parts, pi)

		case pb.OpRecord_DeleteObject:
			di := new(pb.ObjectDeleteInfo)
			err = proto.Unmarshal(data, di)
			if err != nil {
				continue
			}

			if di.ObjectID != ob.ObjectID {
				continue
			}

			ob.deletion = true
		default:
			continue
		}
	}

	ob.ops = of.GetOpRecord()

	// calculated etag
	if len(ob.ObjectInfo.Parts) == 1 {
		ob.ObjectInfo.Etag = ob.ObjectInfo.Parts[0].ETag
	} else if len(ob.ObjectInfo.Parts) > 1 {
		h := md5.New()
		for _, op := range ob.ObjectInfo.Parts {
			h.Write(op.ETag)
		}

		ob.ObjectInfo.Etag = h.Sum(nil)
	}

	return nil
}

func (ob *object) Save(fsID []byte, bucketID uint64, ds store.KVStore) error {
	// to local
	if ob.dirty {
		key := store.NewKey(pb.MetaType_LFS_ObjectInfoKey, fsID, bucketID, ob.ObjectID)
		of := &pb.ObjectForm{
			OpRecord: ob.ops,
		}

		data, err := proto.Marshal(of)
		if err != nil {
			return err
		}
		err = ds.Put(key, data)
		if err != nil {
			return err
		}
		ob.dirty = false
	}
	return nil
}

func loadOpRecord(fsID []byte, bucketID, opID uint64, ds store.KVStore) (*pb.OpRecord, error) {
	key := store.NewKey(pb.MetaType_LFS_OpInfoKey, fsID, bucketID, opID)
	data, err := ds.Get(key)
	if err != nil {
		return nil, err
	}

	or := new(pb.OpRecord)
	err = proto.Unmarshal(data, or)
	if err != nil {
		return nil, err
	}
	return or, nil
}

func saveOpRecord(fsID []byte, bucketID uint64, or *pb.OpRecord, ds store.KVStore) error {
	key := store.NewKey(pb.MetaType_LFS_OpInfoKey, fsID, bucketID, or.OpID)

	data, err := proto.Marshal(or)
	if err != nil {
		return err
	}
	return ds.Put(key, data)
}

// wrap all above
func (l *LfsService) Load() error {
	l.sb.Lock()
	defer l.sb.Unlock()
	err := l.sb.Load(l.fsID, l.ds)
	if err != nil {
		return err
	}

	for i := uint64(0); i < l.sb.NextBucketID; i++ {
		bu := new(bucket)

		bu.Lock()
		err := bu.Load(l.fsID, i, l.ds)
		if err != nil {
			logger.Warn("fail to load bucketID: ", i)
			bu.Unlock()
			continue
		}

		l.sb.buckets[i] = bu

		if !bu.BucketInfo.Deletion {
			l.sb.bucketNameToID[bu.Name] = i

			// load object
			for j := uint64(0); j < bu.NextObjectID; j++ {
				obj := new(object)
				err := obj.Load(l.fsID, i, j, l.ds)
				if err != nil {
					continue
				}

				if !obj.deletion {
					bu.objects.Insert(MetaName(obj.Name), obj)
				}
			}

		}
		bu.Unlock()
	}

	l.sb.ready = true
	return nil
}

func (l *LfsService) Save() error {
	ok := l.sw.TryAcquire(1)
	if ok {
		defer l.sw.Release(1)
	} else {
		return nil
	}

	if !l.Ready() {
		return nil
	}

	err := l.sb.Save(l.fsID, l.ds)
	if err != nil {
		return err
	}

	for _, bucket := range l.sb.buckets {
		err := bucket.Save(l.fsID, l.ds)
		if err != nil {
			logger.Errorf("Flush bucket: %s info failed: %s", &bucket.BucketInfo.Name, err)
		}
	}

	return nil
}

func (l *LfsService) persistMeta() error {
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			if l.Ready() {
				err := l.Save()
				if err != nil {
					logger.Warn("Cannot Persist Meta: ", err)
				}
			}
		case <-l.ctx.Done():
			if l.Ready() {
				err := l.Save()
				if err != nil {
					logger.Warn("Cannot Persist Meta: ", err)
				}
			}
			return nil
		}
	}
}
