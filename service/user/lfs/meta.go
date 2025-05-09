package lfs

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
	lru "github.com/hashicorp/golang-lru"
	rbtree "github.com/sakeven/RbTree"
	"github.com/zeebo/blake3"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/etag"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

type superBlock struct {
	sync.RWMutex
	pb.SuperBlockInfo
	dirty          bool
	write          bool              // set true when finish unfinished jobs
	bucketVerify   uint64            // Get from chain; in case create too mant buckets
	bucketNameToID map[string]uint64 // bucketName -> bucketID
	buckets        []*bucket         // 所有的bucket信息
	etagCache      *lru.ARCCache     // from etags -> objectDigest
}

type bucket struct {
	sync.RWMutex
	types.BucketInfo

	objectTree *rbtree.Tree       // store objects; key is object name; chekc before insert
	objects    map[uint64]*object // key is objectID

	writable bool // false if partial = true
	partial  bool // true if lose local meta
	dirty    bool
}

type object struct {
	sync.RWMutex

	types.ObjectInfo

	ops      []uint64
	deletion bool
	dirty    bool
	pin      bool // state is pin: total=disptach=sent=done
}

type objectDigest struct {
	bucketID uint64
	objectID uint64
}

func newSuperBlock() *superBlock {
	cache, _ := lru.NewARC(1024 * 1024)
	return &superBlock{
		SuperBlockInfo: pb.SuperBlockInfo{
			Version:      0,
			ReplicaNum:   3,
			NextBucketID: 0,
		},
		dirty:          true,
		write:          false,
		bucketVerify:   0,
		buckets:        make([]*bucket, 0, 1),
		bucketNameToID: make(map[string]uint64),
		etagCache:      cache,
	}
}

func (sbl *superBlock) load(userID uint64, ds store.KVStore) error {
	// from local
	key := store.NewKey(pb.MetaType_LFS_SuperBlockInfoKey, userID)
	data, err := ds.Get(key)
	if err != nil {
		// load from state
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

func (sbl *superBlock) save(userID uint64, ds store.KVStore) error {
	// to local
	if sbl.dirty {
		key := store.NewKey(pb.MetaType_LFS_SuperBlockInfoKey, userID)
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

func (l *LfsService) createBucket(bucketID uint64, bucketName string, opts pb.BucketOption) (*bucket, error) {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, bucketID)

	beignHash := blake3.Sum256(buf)
	bi := types.BucketInfo{
		BucketOption: opts,
		BucketInfo: pb.BucketInfo{
			BucketID:     bucketID,
			CTime:        time.Now().Unix(),
			MTime:        time.Now().Unix(),
			Name:         bucketName,
			Deletion:     false,
			Length:       0,
			UsedBytes:    0,
			NextObjectID: 0,
			NextOpID:     0,
			Root:         beignHash[:],
		},
	}

	bu := &bucket{
		BucketInfo: bi,
		dirty:      true,
		objectTree: rbtree.NewTree(),
		objects:    make(map[uint64]*object),
	}

	logger.Debug("push create bucket message")
	tbp := tx.BucketParams{
		BucketOption: opts,
		BucketID:     bucketID,
	}

	data, err := tbp.Serialize()
	if err != nil {
		return nil, err
	}

	msg := &tx.Message{
		Version: 0,
		From:    l.userID,
		To:      l.userID,
		Method:  tx.CreateBucket,
		Params:  data,
	}

	l.msgChan <- msg

	if os.Getenv("MEFS_META_UPLOAD") != "" {
		// send buc meta
		bmp := tx.BucMetaParas{
			BucketID: bucketID,
			Name:     bu.GetName(),
		}

		data, err = bmp.Serialize()
		if err != nil {
			return nil, err
		}
		msg = &tx.Message{
			Version: 0,
			From:    l.userID,
			To:      l.userID,
			Method:  tx.AddBucMeta,
			Params:  data,
		}

		l.msgChan <- msg
	}

	err = bu.saveOptions(l.userID, l.ds)
	if err != nil {
		return nil, err
	}

	// save ops: createOps + setName
	payload, err := proto.Marshal(&opts)
	if err != nil {
		return nil, err
	}

	op := &pb.OpRecord{
		Type:    pb.OpRecord_CreateOption,
		Payload: payload,
	}

	err = l.addOpRecord(bu, op)
	if err != nil {
		return nil, err
	}

	bni := &pb.BucketNameInfo{
		BucketID: bu.BucketID,
		Time:     bu.CTime,
		Name:     bu.Name,
	}

	payload, err = proto.Marshal(bni)
	if err != nil {
		return nil, err
	}

	op = &pb.OpRecord{
		Type:    pb.OpRecord_SetName,
		Payload: payload,
	}

	err = l.addOpRecord(bu, op)
	if err != nil {
		return nil, err
	}

	return bu, nil
}

func (bu *bucket) load(userID uint64, bucketID uint64, ds store.KVStore) error {
	// from local
	key := store.NewKey(pb.MetaType_LFS_BucketOptionKey, userID, bucketID)
	data, err := ds.Get(key)
	if err != nil {
		return err
	}

	bo := new(pb.BucketOption)
	err = proto.Unmarshal(data, bo)
	if err != nil {
		return err
	}

	key = store.NewKey(pb.MetaType_LFS_BucketInfoKey, userID, bucketID)
	data, err = ds.Get(key)
	if err != nil {
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
	bu.objectTree = rbtree.NewTree()
	bu.objects = make(map[uint64]*object)

	return nil
}

func (bu *bucket) save(userID uint64, ds store.KVStore) error {
	// to local
	if bu.dirty {
		key := store.NewKey(pb.MetaType_LFS_BucketInfoKey, userID, bu.BucketID)
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

func (bu *bucket) saveOptions(userID uint64, ds store.KVStore) error {
	key := store.NewKey(pb.MetaType_LFS_BucketOptionKey, userID, bu.BucketID)
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

func (l *LfsService) addOpRecord(bu *bucket, por *pb.OpRecord) error {
	por.OpID = bu.NextOpID

	key := store.NewKey(pb.MetaType_LFS_OpInfoKey, l.userID, bu.BucketID, por.OpID)
	data, err := proto.Marshal(por)
	if err != nil {
		return err
	}
	err = l.ds.Put(key, data)
	if err != nil {
		return err
	}

	bu.NextOpID++

	nh := blake3.New()
	nh.Write(bu.Root)
	nh.Write(data)
	bu.Root = nh.Sum(nil)

	bu.MTime = time.Now().Unix()
	bu.dirty = true

	err = bu.save(l.userID, l.ds)
	if err != nil {
		return err
	}

	//go l.broadcast(l.userID, bu.BucketID, por.OpID)

	return nil
}

func NewObject() *object {
	return &object{
		ObjectInfo: types.ObjectInfo{
			Parts: make([]*pb.ObjectPartInfo, 0, 1),
		},
	}
}

func (ob *object) load(userID uint64, bucketID, objectID uint64, ds store.KVStore) error {
	// from local
	key := store.NewKey(pb.MetaType_LFS_ObjectInfoKey, userID, bucketID, objectID)
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
		or, err := loadOpRecord(userID, bucketID, opID, ds)
		if err != nil {
			return err
		}

		logger.Debug("load object ops: ", bucketID, objectID, opID, or.GetType())

		switch or.GetType() {
		case pb.OpRecord_CreateObject:
			oi := new(pb.ObjectInfo)
			err = proto.Unmarshal(or.GetPayload(), oi)
			if err != nil {
				logger.Debug("load object ops err: ", objectID, opID, or.GetType(), err)
				continue
			}

			ob.ObjectInfo.ObjectInfo = *oi
			ob.ObjectInfo.Mtime = oi.Time

		case pb.OpRecord_AddData:
			pi := new(pb.ObjectPartInfo)
			err = proto.Unmarshal(or.GetPayload(), pi)
			if err != nil {
				logger.Debug("load object ops err: ", objectID, opID, or.GetType(), err)
				continue
			}

			if pi.ObjectID != ob.ObjectID {
				logger.Debug("load object ops err: ", objectID, opID, or.GetType(), pi.ObjectID, ob.ObjectID)
				continue
			}

			ob.addPartInfo(pi)
		case pb.OpRecord_DeleteObject:
			di := new(pb.ObjectDeleteInfo)
			err = proto.Unmarshal(or.GetPayload(), di)
			if err != nil {
				logger.Debug("load object ops err: ", objectID, opID, or.GetType(), err)
				continue
			}

			if di.ObjectID != ob.ObjectID {
				logger.Debug("load object ops err: ", objectID, opID, or.GetType(), di.ObjectID, ob.ObjectID)
				continue
			}

			ob.deletion = true
		case pb.OpRecord_Rename:
			cni := new(pb.ObjectRenameInfo)
			err = proto.Unmarshal(or.GetPayload(), cni)
			if err != nil {
				logger.Debug("load object ops err: ", objectID, opID, or.GetType(), err)
				continue
			}
			ob.Name = cni.GetName()
		}
	}

	ob.ops = of.GetOpRecord()

	return nil
}

func (ob *object) addPartInfo(opi *pb.ObjectPartInfo) error {
	ob.Parts = append(ob.Parts, opi)

	/*
		if len(ob.ObjectInfo.Parts) == 1 {
			newTag := make([]byte, len(opi.ETag))
			copy(newTag, opi.ETag)
			ob.ObjectInfo.ETag = newTag
		} else {
			newEtag, err := xor(ob.ObjectInfo.ETag, opi.ETag)
			if err != nil {
				return err
			}

			ob.ObjectInfo.ETag = newEtag
		}
	*/

	newTag := make([]byte, len(opi.ETag))
	copy(newTag, opi.ETag)
	ob.ObjectInfo.ETag = newTag

	ob.Size += opi.GetLength() // record object raw length acc
	ob.StoredBytes += opi.GetStoredBytes()
	if ob.Mtime < opi.GetTime() {
		ob.Mtime = opi.GetTime()
	}

	return nil
}

// after save, object is clean
func (ob *object) Save(userID uint64, ds store.KVStore) error {
	// to local
	if ob.dirty {
		key := store.NewKey(pb.MetaType_LFS_ObjectInfoKey, userID, ob.BucketID, ob.ObjectID)
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

func loadOpRecord(userID uint64, bucketID, opID uint64, ds store.KVStore) (*pb.OpRecord, error) {
	key := store.NewKey(pb.MetaType_LFS_OpInfoKey, userID, bucketID, opID)
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

// wrap all above load
func (l *LfsService) load() error {
	l.sb.Lock()
	defer l.sb.Unlock()
	// 1. load super block
	err := l.sb.load(l.userID, l.ds)
	if err != nil {
		logger.Warn("fail to load super block: ", err)
		l.getSBFromState()
	}

	// 2. load each bucket
	for i := uint64(0); i < l.sb.NextBucketID; i++ {
		bu := new(bucket)
		bu.Deletion = true
		bu.BucketID = i

		l.sb.buckets[i] = bu

		bu.Lock()
		err := bu.load(l.userID, i, l.ds)
		if err != nil {
			bu.Unlock()
			logger.Warn("fail to load bucketID: ", i, err)

			// get from state
			l.getBucketOptsFromState(bu)

			continue
		}

		logger.Debug("load bucket: ", i, bu.BucketInfo)

		if !bu.BucketInfo.Deletion {
			l.sb.bucketNameToID[bu.Name] = i

			// 3. load objects
			for j := uint64(0); j < bu.NextObjectID; j++ {
				logger.Debug("load object: ", j)
				obj := NewObject()
				err := obj.load(l.userID, i, j, l.ds)
				if err != nil {
					continue
				}

				if !obj.deletion {
					tt, dist, donet, ct := 0, 0, 0, 0
					for _, opID := range obj.ops[1 : 1+len(obj.Parts)] {
						total, dis, done, c := l.OrderMgr.GetSegJogState(obj.BucketID, opID)
						dist += dis
						donet += done
						tt += total
						ct += c
					}

					obj.State = fmt.Sprintf("total: %d, dispatch: %d, sent: %d, confirm: %d", tt, dist, donet, ct)
					if tt > 0 && tt == dist && tt == donet && tt == ct {
						obj.pin = true
					}

					if obj.Name == "" {
						newName, err := etag.ToString(obj.ETag)
						if err != nil {
							continue
						}
						obj.Name = newName
					}

					ename, err := etag.ToString(obj.ETag)
					if err == nil {
						od := &objectDigest{
							bucketID: obj.BucketID,
							objectID: obj.ObjectID,
						}
						l.sb.etagCache.Add(ename, od)
					}

					bu.objects[obj.ObjectID] = obj
					if bu.objectTree.Find(MetaName(obj.Name)) == nil {
						bu.objectTree.Insert(MetaName(obj.Name), obj)
					}
				}
			}

		}
		bu.Unlock()
	}
	return nil
}

func (l *LfsService) getSBFromState() error {
	nextBucket, err := l.StateGetBucketAt(l.ctx, l.userID)
	if err != nil || nextBucket == 0 {
		return err
	}

	l.sb.NextBucketID = nextBucket
	l.sb.buckets = make([]*bucket, nextBucket)
	return nil
}

func (l *LfsService) getBucketOptsFromState(bu *bucket) error {
	// from state
	bo, err := l.StateGetBucOpt(l.ctx, l.userID, bu.BucketID)
	if err != nil {
		return err
	}

	bu.BucketInfo.BucketOption = *bo
	bu.Deletion = false
	bu.partial = true

	bu.Name = fmt.Sprintf("bucket-%d", bu.BucketID)
	bmp, err := l.StateGetBucMeta(l.ctx, l.userID, bu.BucketID)
	if err == nil && bmp.Name != "" {
		bu.Name = bmp.Name
	}

	l.sb.bucketNameToID[bu.Name] = bu.BucketID

	bu.objectTree = rbtree.NewTree()
	bu.objects = make(map[uint64]*object)

	go l.getObjectFromState(bu)

	return nil
}

func (l *LfsService) getObjectFromState(bu *bucket) {
	if os.Getenv("MEFS_RECOVERY_MODE") == "" {
		return
	}
	for i := uint64(0); ; i++ {
		omv, err := l.StateGetObjMeta(l.ctx, l.userID, bu.BucketID, i)
		if err != nil {
			return
		}

		poi := pb.ObjectInfo{
			ObjectID:    i,
			BucketID:    bu.BucketID,
			Name:        omv.Name,
			Encryption:  omv.Encrypt,
			UserDefined: make(map[string]string),
		}

		poi.UserDefined["nencryption"] = omv.NEncrypt

		if len(omv.Extra) >= 8 {
			csize := binary.BigEndian.Uint64(omv.Extra[:8])
			poi.UserDefined["etag"] = "cid-" + strconv.FormatUint(csize, 10)
		}

		obj := &object{
			ObjectInfo: types.ObjectInfo{
				ObjectInfo: poi,
				Size:       omv.Length,
				ETag:       omv.ETag,
				Parts:      make([]*pb.ObjectPartInfo, 0, 1),
				State:      fmt.Sprintf("user: %d", l.userID),
			},
			ops:      make([]uint64, 0, 2),
			deletion: false,
			pin:      true,
		}

		opi := &pb.ObjectPartInfo{
			Offset: omv.Offset,
			Length: omv.Length,
			ETag:   omv.ETag,
		}

		obj.Parts = append(obj.Parts, opi)

		if obj.Name == "" {
			newName, err := etag.ToString(obj.ETag)
			if err != nil {
				continue
			}
			obj.Name = newName
		}

		ename, err := etag.ToString(obj.ETag)
		if err == nil {
			od := &objectDigest{
				bucketID: obj.BucketID,
				objectID: obj.ObjectID,
			}
			l.sb.etagCache.Add(ename, od)
		}

		bu.objects[obj.ObjectID] = obj
		if bu.objectTree.Find(MetaName(obj.Name)) == nil {
			bu.objectTree.Insert(MetaName(obj.Name), obj)
		}
		bu.NextObjectID = obj.ObjectID + 1

		if bu.DataCount != 0 {
			stripeCnt := (omv.Length-1)/(build.DefaultSegSize*uint64(bu.DataCount)) + 1
			bu.Length += stripeCnt * uint64(bu.DataCount) * build.DefaultSegSize
			bu.UsedBytes += stripeCnt * uint64(bu.DataCount+bu.ParityCount) * build.DefaultSegSize
		}
	}
}

func (l *LfsService) save() error {
	err := l.sb.save(l.userID, l.ds)
	if err != nil {
		return err
	}

	for _, bucket := range l.sb.buckets {
		err := bucket.save(l.userID, l.ds)
		if err != nil {
			logger.Errorf("flush bucket: %s failed: %s", &bucket.BucketInfo.Name, err)
		}
	}

	return nil
}

func (l *LfsService) persistMeta() {
	tick := time.NewTicker(60 * time.Second)
	defer tick.Stop()

	ltick := time.NewTicker(1800 * time.Second)
	defer ltick.Stop()
	for {
		select {
		case <-l.readyChan:
			l.sb.write = true
			logger.Debug("lfs is ready for write")
		case bid := <-l.bucketChan:
			logger.Debug("lfs bucket is verified: ", bid)
			l.sb.Lock()
			if bid < l.sb.NextBucketID {
				if l.sb.bucketVerify <= bid {
					l.sb.bucketVerify = bid + 1
				}

				bu := l.sb.buckets[bid]
				bu.Confirmed = true
				// register in order
				go l.registerBucket(bu.BucketID, bu.NextOpID, &bu.BucketOption)
			}
			l.sb.Unlock()
		case bid := <-l.bucketReadyChan:
			logger.Debugf("lfs bucket %d is ready for wirte", bid)
			l.sb.Lock()
			bu := l.sb.buckets[bid]
			if !bu.partial {
				bu.writable = true
			}
			l.sb.Unlock()
		case <-tick.C:
			if l.Writeable() {
				err := l.save()
				if err != nil {
					logger.Warn("cannot persist meta: ", err)
				}
			}
		case <-ltick.C:
			if l.Writeable() {
				l.getPayInfo()
			}
		case <-l.ctx.Done():
			if l.Writeable() {
				err := l.save()
				if err != nil {
					logger.Warn("Cannot Persist Meta: ", err)
				}
			}
			return
		}
	}
}

func (l *LfsService) registerBucket(bucketID, nextOpID uint64, bopt *pb.BucketOption) {
	l.OrderMgr.RegisterBucket(bucketID, nextOpID, bopt)
	l.bucketReadyChan <- bucketID
}

func (l *LfsService) getPayInfo() {
	// query regular
	pi, err := l.OrderMgr.OrderGetPayInfoAt(l.ctx, 0)
	if err == nil {
		np := pi.NeedPay.Sub(pi.NeedPay, pi.Paid)
		np.Mul(np, big.NewInt(105))
		np.Div(np, big.NewInt(100))
		l.needPay.Set(np)
		l.bal.Set(pi.Balance)
	}
}

// reconstruct related

func (l *LfsService) Recontruct() error {
	l.sb.Lock()
	defer l.sb.Unlock()
	// 1. load super block
	l.sb.load(l.userID, l.ds)

	// 2. load each bucket
	for i := uint64(0); i < l.sb.NextBucketID; i++ {
		bu := new(bucket)
		bu.BucketID = i
		bu.objectTree = rbtree.NewTree()
		bu.objects = make(map[uint64]*object)

		l.sb.buckets[i] = bu

		bu.Lock()
		err := bu.recontruct(l.userID, l.ds)
		if err != nil {
			logger.Warn("fail to load bucketID: ", i, err)
			bu.Unlock()
			continue
		}

		logger.Debug("load bucket: ", i, bu.BucketInfo)

		if !bu.BucketInfo.Deletion {
			l.sb.bucketNameToID[bu.Name] = i

			// 3. load objects
			for j := uint64(0); j < bu.NextObjectID; j++ {
				logger.Debug("load object: ", j)
				obj := bu.objects[j]

				if !obj.deletion {
					tt, dist, donet, ct := 0, 0, 0, 0
					for _, opID := range obj.ops[1 : 1+len(obj.Parts)] {
						total, dis, done, c := l.OrderMgr.GetSegJogState(obj.BucketID, opID)
						dist += dis
						donet += done
						tt += total
						ct += c
					}

					obj.State = fmt.Sprintf("total: %d, dispatch: %d, sent: %d, confirm: %d", tt, dist, donet, ct)
					if tt > 0 && tt == dist && tt == donet && tt == ct {
						obj.pin = true
					}

					if obj.Name == "" {
						newName, err := etag.ToString(obj.ETag)
						if err != nil {
							continue
						}
						obj.Name = newName
					}

					ename, err := etag.ToString(obj.ETag)
					if err == nil {
						od := &objectDigest{
							bucketID: obj.BucketID,
							objectID: obj.ObjectID,
						}
						l.sb.etagCache.Add(ename, od)
					}
				}
			}

		}
		bu.Unlock()
	}
	return nil
}

func (bu *bucket) recontruct(userID uint64, ds store.KVStore) error {
	nh := blake3.New()
	opID := uint64(0)
	for {
		key := store.NewKey(pb.MetaType_LFS_OpInfoKey, userID, bu.BucketID, opID)
		data, err := ds.Get(key)
		if err != nil {
			return nil
		}

		or := new(pb.OpRecord)
		err = proto.Unmarshal(data, or)
		if err != nil {
			return err
		}

		err = bu.loadOp(or)
		if err != nil {
			return err
		}

		if bu.NextOpID != or.OpID {
			return xerrors.Errorf("opID mismatch at %d", bu.NextOpID)
		}

		bu.NextOpID++

		nh.Write(bu.Root)
		nh.Write(data)
		bu.Root = nh.Sum(nil)

		opID++
	}
}

func (bu *bucket) loadOp(or *pb.OpRecord) error {
	logger.Debug("load object ops: ", bu.BucketID, or.GetOpID(), or.GetType())

	switch or.GetType() {
	case pb.OpRecord_CreateObject:
		oi := new(pb.ObjectInfo)
		err := proto.Unmarshal(or.GetPayload(), oi)
		if err != nil {
			return err
		}

		obj := &object{
			ObjectInfo: types.ObjectInfo{
				ObjectInfo: *oi,
				Parts:      make([]*pb.ObjectPartInfo, 0, 1),
				Mtime:      oi.Time,
			},
			ops: make([]uint64, 0, 2),
		}
		obj.ops = append(obj.ops, or.OpID)

		bu.objects[obj.ObjectID] = obj
		if bu.objectTree.Find(MetaName(obj.Name)) == nil {
			bu.objectTree.Insert(MetaName(obj.Name), obj)
		}
		bu.NextObjectID = obj.ObjectID + 1
		bu.MTime = oi.GetTime()

	case pb.OpRecord_AddData:
		pi := new(pb.ObjectPartInfo)
		err := proto.Unmarshal(or.GetPayload(), pi)
		if err != nil {
			return err
		}

		logger.Debug("load: ", pi.Offset, pi.Length, pi.StoredBytes)

		obj := bu.objects[pi.ObjectID]
		obj.addPartInfo(pi)
		obj.ops = append(obj.ops, or.OpID)
		// update size in bucket
		if bu.Length != pi.Offset {
			return xerrors.Errorf("mismatch start %d %d", bu.Length, pi.Offset)
		}

		bu.MTime = pi.GetTime()

		if bu.DataCount != 0 {
			stripeCnt := (pi.Length-1)/(build.DefaultSegSize*uint64(bu.DataCount)) + 1
			bu.Length += stripeCnt * uint64(bu.DataCount) * build.DefaultSegSize
			bu.UsedBytes += stripeCnt * uint64(bu.DataCount+bu.ParityCount) * build.DefaultSegSize
		}

	case pb.OpRecord_DeleteObject:
		di := new(pb.ObjectDeleteInfo)
		err := proto.Unmarshal(or.GetPayload(), di)
		if err != nil {
			return err
		}

		obj := bu.objects[di.ObjectID]
		obj.deletion = true
		bu.objectTree.Delete(MetaName(obj.Name))
		delete(bu.objects, obj.ObjectID)

		obj.ops = append(obj.ops, or.OpID)

		bu.MTime = di.GetTime()

	case pb.OpRecord_Rename:
		cni := new(pb.ObjectRenameInfo)
		err := proto.Unmarshal(or.GetPayload(), cni)
		if err != nil {
			return err
		}
		obj := bu.objects[cni.ObjectID]

		bu.objectTree.Delete(MetaName(obj.Name))
		obj.Name = cni.GetName()
		bu.objectTree.Insert(MetaName(cni.GetName()), obj)

		obj.ops = append(obj.ops, or.OpID)

	case pb.OpRecord_CreateOption:
		bo := new(pb.BucketOption)
		err := proto.Unmarshal(or.GetPayload(), bo)
		if err != nil {
			return err
		}

		bu.BucketOption = *bo
		bu.CTime = or.Time
		bu.MTime = or.Time

	case pb.OpRecord_SetName:
		bni := new(pb.BucketNameInfo)
		err := proto.Unmarshal(or.GetPayload(), bni)
		if err != nil {
			return err
		}

		bu.Name = bni.Name
		bu.MTime = bni.Time

	default:
		return xerrors.Errorf("unsupported type: %d", or.GetType())
	}
	return nil
}
