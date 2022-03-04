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

	if l.needPay.Cmp(l.bal) > 0 {
		return nil, xerrors.Errorf("not have enough balance, please rcharge at least %d", l.needPay.Sub(l.needPay, l.bal))
	}

	logger.Debugf("Upload object: %s to bucket: %s begin", objectName, bucketName)

	bucket, err := l.getBucketInfo(bucketName)
	if err != nil {
		return nil, err
	}

	bucket.Lock()
	defer bucket.Unlock()

	object, err := l.createObject(ctx, bucket, objectName, opts)
	if err != nil {
		return nil, err
	}

	object.Lock()
	defer object.Unlock()

	nt := time.Now()
	err = l.upload(ctx, bucket, object, reader)
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

func (l *LfsService) createObject(ctx context.Context, bucket *bucket, objectName string, opts *types.PutObjectOptions) (*object, error) {
	objectElement := bucket.objects.Find(MetaName(objectName))
	if objectElement != nil {
		return nil, ErrObjectAlreadyExist
	}

	// 1. save op
	// 2. save bucket
	// 3. save object

	poi := pb.ObjectInfo{
		ObjectID:   bucket.NextObjectID,
		BucketID:   bucket.BucketID,
		Time:       time.Now().Unix(),
		Name:       objectName,
		Encryption: "aes", // todo, from options
	}

	payload, err := proto.Marshal(&poi)
	if err != nil {
		return nil, err
	}

	op := &pb.OpRecord{
		Type:    pb.OpRecord_CreateObject,
		Payload: payload,
	}

	bucket.NextObjectID++
	err = bucket.addOpRecord(l.userID, op, l.ds)
	if err != nil {
		return nil, err
	}

	// object 实例
	object := &object{
		ObjectInfo: types.ObjectInfo{
			ObjectInfo: poi,
			Parts:      make([]*pb.ObjectPartInfo, 0, 1),
		},
		ops:      make([]uint64, 0, 2),
		deletion: false,
	}

	object.ops = append(object.ops, op.OpID)
	object.dirty = true
	err = object.Save(l.userID, l.ds)
	if err != nil {
		return nil, err
	}

	bucket.objects.Insert(MetaName(objectName), object)

	logger.Debugf("Upload create object: %s in bucket: %s", object.GetName(), bucket.GetName(), op.OpID)

	return object, nil
}
