package lfs

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/utils/etag"
)

func (l *LfsService) PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, opts types.PutObjectOptions) (types.ObjectInfo, error) {
	oi := types.ObjectInfo{}
	ok := l.sw.TryAcquire(10)
	if !ok {
		return oi, ErrResourceUnavailable
	}
	defer l.sw.Release(10)

	if !l.Writeable() {
		return oi, ErrLfsReadOnly
	}

	// verify balance
	if l.needPay.Cmp(l.bal) > 0 {
		return oi, xerrors.Errorf("not have enough balance, please rcharge at least %d", l.needPay.Sub(l.needPay, l.bal))
	}

	replaceName := false
	if objectName == "" {
		replaceName = true
		objectName = uuid.NewString()
	}

	err := checkObjectName(objectName)
	if err != nil {
		return oi, xerrors.Errorf("object name is invalid: %s", err)
	}

	logger.Debugf("Upload object: %s to bucket: %s begin", objectName, bucketName)

	// get bucket with bucket name
	bucket, err := l.getBucketInfo(bucketName)
	if err != nil {
		return oi, err
	}

	if bucket.BucketID >= l.sb.bucketVerify {
		return oi, xerrors.Errorf("bucket %d is confirming", bucket.BucketID)
	}

	bucket.Lock()
	defer bucket.Unlock()

	// create new object and insert into rbtree
	object, err := l.createObject(ctx, bucket, objectName, opts)
	if err != nil {
		return oi, err
	}

	object.Lock()
	defer object.Unlock()

	nt := time.Now()

	// upload object into bucket
	err = l.upload(ctx, bucket, object, reader, opts)
	if err != nil {
		return object.ObjectInfo, err
	}

	if replaceName {
		// replace name
		newName, err := etag.ToString(object.ETag)
		if err != nil {
			return object.ObjectInfo, err
		}
		l.renameObject(ctx, bucket, object, newName)
	}

	if len(object.ETag) != md5.Size {
		ename, _ := etag.ToString(object.ETag)
		l.sb.cids[ename] = &objectDigest{
			bucketID: object.BucketID,
			objectID: object.ObjectID,
		}
	}

	tt, dist, donet, ct := 0, 0, 0, 0
	for _, opID := range object.ops[1 : 1+len(object.Parts)] {
		total, dis, done, c := l.OrderMgr.GetSegJogState(object.BucketID, opID)
		dist += dis
		donet += done
		tt += total
		ct += c
	}

	if tt > 0 && tt == dist && tt == donet && tt == ct {
		object.pin = true
	}
	object.State = fmt.Sprintf("total: %d, dispatch: %d, sent: %d, confirm: %d", tt, dist, donet, ct)

	logger.Debugf("Upload object: %s to bucket: %s end, cost: %s", objectName, bucketName, time.Since(nt))

	return object.ObjectInfo, nil
}

// create object with bucket, object name, opts
func (l *LfsService) createObject(ctx context.Context, bucket *bucket, objectName string, opts types.PutObjectOptions) (*object, error) {
	// check if object exists in rbtree
	objectElement := bucket.objectTree.Find(MetaName(objectName))
	if objectElement != nil {
		return nil, xerrors.Errorf("object %s already exist", objectName)
	}

	// 1. save op
	// 2. save bucket
	// 3. save object

	// new object info
	poi := pb.ObjectInfo{
		ObjectID:   bucket.NextObjectID,
		BucketID:   bucket.BucketID,
		Time:       time.Now().Unix(),
		Name:       objectName,
		Encryption: "none",
	}

	if opts.UserDefined != nil {
		val, ok := opts.UserDefined["encryption"]
		if ok {
			poi.Encryption = val
		}
		poi.UserDefined = opts.UserDefined
	}

	// serialize
	payload, err := proto.Marshal(&poi)
	if err != nil {
		return nil, err
	}

	op := &pb.OpRecord{
		Type:    pb.OpRecord_CreateObject,
		Payload: payload,
	}

	// update objectID in bucket
	bucket.NextObjectID++
	err = bucket.addOpRecord(l.userID, op, l.ds)
	if err != nil {
		return nil, err
	}

	// new object instance
	object := &object{
		ObjectInfo: types.ObjectInfo{
			ObjectInfo: poi,
			Parts:      make([]*pb.ObjectPartInfo, 0, 1),
		},
		ops:      make([]uint64, 0, 2),
		deletion: false,
	}

	// save op
	object.ops = append(object.ops, op.OpID)
	object.dirty = true
	// save object ops
	err = object.Save(l.userID, l.ds)
	if err != nil {
		return nil, err
	}

	// insert new object into rbtree of bucket
	bucket.objectTree.Insert(MetaName(objectName), object)
	bucket.objects[object.ObjectID] = object

	logger.Debugf("Upload create object: %s in bucket: %s %d", object.GetName(), bucket.GetName(), op.OpID)

	return object, nil
}

func (l *LfsService) renameObject(ctx context.Context, bucket *bucket, object *object, newName string) error {
	err := checkObjectName(newName)
	if err != nil {
		return xerrors.Errorf("object re name is invalid: %s", err)
	}

	poi := pb.ObjectRenameInfo{
		ObjectID: object.ObjectID,
		Name:     newName,
	}

	// serialize
	payload, err := proto.Marshal(&poi)
	if err != nil {
		return err
	}

	op := &pb.OpRecord{
		Type:    pb.OpRecord_Rename,
		Payload: payload,
	}

	err = bucket.addOpRecord(l.userID, op, l.ds)
	if err != nil {
		return err
	}

	object.Name = newName

	// save op
	object.ops = append(object.ops, op.OpID)
	object.dirty = true
	// save object
	err = object.Save(l.userID, l.ds)
	if err != nil {
		return err
	}

	// remove old
	bucket.objectTree.Delete(MetaName(object.Name))

	// insert new object into rbtree of bucket
	object.Name = newName
	bucket.objectTree.Insert(MetaName(newName), object)

	logger.Debugf("object %d rename to: %s in bucket: %s %d", object.ObjectID, object.GetName(), bucket.GetName(), op.OpID)

	return nil
}
