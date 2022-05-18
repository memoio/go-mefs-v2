package lfs

import (
	"context"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
	rbtree "github.com/sakeven/RbTree"
	"github.com/zeebo/blake3"

	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/lib/utils/etag"
)

type superBlock struct {
	sync.RWMutex
	pb.SuperBlockInfo
	dirty          bool
	write          bool                     // set true when finish unfinished jobs
	bucketVerify   uint64                   // Get from chain; in case create too mant buckets
	bucketNameToID map[string]uint64        // bucketName -> bucketID
	buckets        []*bucket                // 所有的bucket信息
	cids           map[string]*objectDigest // from cid -> object
}

type bucket struct {
	sync.RWMutex
	types.BucketInfo

	objectTree *rbtree.Tree       // store objects; key is object name
	objects    map[uint64]*object // key is objectID

	dirty bool
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
	return &superBlock{
		SuperBlockInfo: pb.SuperBlockInfo{
			Version:      0,
			ReplicaNum:   3,
			NextBucketID: 0,
		},
		dirty:          true,
		write:          false,
		bucketVerify:   0,
		bucketNameToID: make(map[string]uint64),
		cids:           make(map[string]*objectDigest),
	}
}

func (sbl *superBlock) Load(userID uint64, ds store.KVStore) error {
	// from local
	key := store.NewKey(pb.MetaType_LFS_SuperBlockInfoKey, userID)
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

func (sbl *superBlock) Save(userID uint64, ds store.KVStore) error {
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

	// handle result and retry?

	var mid types.MsgID
	retry := 0
	for retry < 60 {
		retry++
		id, err := l.OrderMgr.PushMessage(l.ctx, msg)
		if err != nil {
			time.Sleep(10 * time.Second)
			continue
		}
		mid = id
		break
	}

	go func(bucketID uint64, mid types.MsgID) {
		ctx, cancle := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancle()
		logger.Debug("waiting tx message done: ", mid)

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			st, err := l.OrderMgr.SyncGetTxMsgStatus(ctx, mid)
			if err != nil {
				time.Sleep(5 * time.Second)
				continue
			}

			if st.Status.Err == 0 {
				logger.Debug("tx message done success: ", mid, msg.From, msg.To, msg.Method, st.BlockID, st.Height)
				l.bucketChan <- bucketID
			} else {
				logger.Warn("tx message done fail: ", mid, msg.From, msg.To, msg.Method, st.BlockID, st.Height, st.Status)
			}

			break
		}
	}(bucketID, mid)

	err = bu.SaveOptions(l.userID, l.ds)
	if err != nil {
		return nil, err
	}

	err = bu.Save(l.userID, l.ds)
	if err != nil {
		return nil, err
	}

	return bu, nil
}

func (bu *bucket) Load(userID uint64, bucketID uint64, ds store.KVStore) error {
	// from local
	key := store.NewKey(pb.MetaType_LFS_BucketOptionKey, userID, bucketID)
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

	key = store.NewKey(pb.MetaType_LFS_BucketInfoKey, userID, bucketID)
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
	bu.objectTree = rbtree.NewTree()
	bu.objects = make(map[uint64]*object)

	return nil
}

func (bu *bucket) Save(userID uint64, ds store.KVStore) error {
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

func (bu *bucket) SaveOptions(userID uint64, ds store.KVStore) error {
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

func (bu *bucket) addOpRecord(userID uint64, por *pb.OpRecord, ds store.KVStore) error {
	por.OpID = bu.NextOpID

	key := store.NewKey(pb.MetaType_LFS_OpInfoKey, userID, bu.BucketID, por.OpID)
	data, err := proto.Marshal(por)
	if err != nil {
		return err
	}
	err = ds.Put(key, data)
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

	err = bu.Save(userID, ds)
	if err != nil {
		return err
	}

	return nil
}

func NewObject() *object {
	return &object{
		ObjectInfo: types.ObjectInfo{
			Parts: make([]*pb.ObjectPartInfo, 0, 1),
		},
	}
}

func (ob *object) Load(userID uint64, bucketID, objectID uint64, ds store.KVStore) error {
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
				logger.Debug("load object ops err:", objectID, opID, or.GetType(), err)
				continue
			}

			ob.ObjectInfo.ObjectInfo = *oi
			ob.ObjectInfo.Mtime = oi.Time

		case pb.OpRecord_AddData:
			pi := new(pb.ObjectPartInfo)
			err = proto.Unmarshal(or.GetPayload(), pi)
			if err != nil {
				logger.Debug("load object ops err:", objectID, opID, or.GetType(), err)
				continue
			}

			if pi.ObjectID != ob.ObjectID {
				logger.Debug("load object ops fail:", objectID, opID, or.GetType(), pi.ObjectID, ob.ObjectID)
				continue
			}

			ob.addPartInfo(pi)
		case pb.OpRecord_DeleteObject:
			di := new(pb.ObjectDeleteInfo)
			err = proto.Unmarshal(or.GetPayload(), di)
			if err != nil {
				logger.Debug("load object ops err:", objectID, opID, or.GetType(), err)
				continue
			}

			if di.ObjectID != ob.ObjectID {
				logger.Debug("load object ops fail:", objectID, opID, or.GetType(), di.ObjectID, ob.ObjectID)
				continue
			}

			ob.deletion = true
		case pb.OpRecord_Rename:
			cni := new(pb.ObjectRenameInfo)
			err = proto.Unmarshal(or.GetPayload(), cni)
			if err != nil {
				logger.Debug("load object ops err:", objectID, opID, or.GetType(), err)
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
	l.sb.Load(l.userID, l.ds)

	// 2. load each bucket
	for i := uint64(0); i < l.sb.NextBucketID; i++ {
		bu := new(bucket)
		bu.Deletion = true

		l.sb.buckets[i] = bu

		bu.Lock()
		err := bu.Load(l.userID, i, l.ds)
		if err != nil {
			bu.Unlock()
			logger.Warn("fail to load bucketID: ", i)
			continue
		}

		logger.Debug("load bucket: ", i, bu.BucketInfo)

		if !bu.BucketInfo.Deletion {
			l.sb.bucketNameToID[bu.Name] = i

			// 3. load objects
			for j := uint64(0); j < bu.NextObjectID; j++ {
				logger.Debug("load object: ", j)
				obj := NewObject()
				err := obj.Load(l.userID, i, j, l.ds)
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

					if len(obj.ETag) != md5.Size {
						ename, _ := etag.ToString(obj.ETag)
						l.sb.cids[ename] = &objectDigest{
							bucketID: obj.BucketID,
							objectID: obj.ObjectID,
						}
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

func (l *LfsService) save() error {
	err := l.sb.Save(l.userID, l.ds)
	if err != nil {
		return err
	}

	for _, bucket := range l.sb.buckets {
		err := bucket.Save(l.userID, l.ds)
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
				go l.OrderMgr.RegisterBucket(bu.BucketID, bu.NextOpID, &bu.BucketOption)
			}
			l.sb.Unlock()
		case <-tick.C:
			if l.Writeable() {
				err := l.save()
				if err != nil {
					logger.Warn("Cannot Persist Meta: ", err)
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
