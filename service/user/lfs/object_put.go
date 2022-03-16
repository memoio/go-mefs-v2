package lfs

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/gogo/protobuf/proto"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
)

func (l *LfsService) PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, opts *types.PutObjectOptions) (*types.ObjectInfo, error) {
	ok := l.sw.TryAcquire(10)
	if !ok {
		return nil, ErrResourceUnavailable
	}
	defer l.sw.Release(10)

	if !l.Ready() {
		return nil, ErrLfsServiceNotReady
	}

	if !l.Writeable() {
		return nil, ErrLfsReadOnly
	}

	// verify balance
	if l.needPay.Cmp(l.bal) > 0 {
		return nil, xerrors.Errorf("not have enough balance, please rcharge at least %d", l.needPay.Sub(l.needPay, l.bal))
	}

	logger.Debugf("Upload object: %s to bucket: %s begin", objectName, bucketName)

	// get bucket with bucket name
	bucket, err := l.getBucketInfo(bucketName)
	if err != nil {
		return nil, err
	}

	bucket.Lock()
	defer bucket.Unlock()

	// create new object and insert into rbtree
	object, err := l.createObject(ctx, bucket, objectName, opts)
	if err != nil {
		return nil, err
	}

	object.Lock()
	defer object.Unlock()

	nt := time.Now()

	// upload object into bucket
	err = l.upload(ctx, bucket, object, reader, opts)
	if err != nil {
		return &object.ObjectInfo, err
	}

	tt, dist, donet, ct := 0, 0, 0, 0
	for _, opID := range object.ops[1:] {
		total, dis, done, c := l.OrderMgr.GetSegJogState(object.BucketID, opID)
		dist += dis
		donet += done
		tt += total
		ct += c
	}

	object.State = fmt.Sprintf("total %d, dispatch %d, done %d, confirm %d", tt, dist, donet, ct)

	logger.Debugf("Upload object: %s to bucket: %s end, cost: %s", objectName, bucketName, time.Since(nt))

	return &object.ObjectInfo, nil
}

// create object with bucket, object name, opts
func (l *LfsService) createObject(ctx context.Context, bucket *bucket, objectName string, opts *types.PutObjectOptions) (*object, error) {
	// check if object exists in rbtree
	objectElement := bucket.objects.Find(MetaName(objectName))
	if objectElement != nil {
		return nil, ErrObjectAlreadyExist
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
		Encryption: "aes", // todo, from options
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
	// clean object
	err = object.Save(l.userID, l.ds)
	if err != nil {
		return nil, err
	}

	// insert new object into rbtree of bucket
	bucket.objects.Insert(MetaName(objectName), object)

	logger.Debugf("Upload create object: %s in bucket: %s %d", object.GetName(), bucket.GetName(), op.OpID)

	return object, nil
}
