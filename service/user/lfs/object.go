package lfs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
)

func (l *LfsService) getObjectInfo(bu *bucket, objectName string) (*object, error) {
	if !l.Ready() {
		return nil, ErrLfsServiceNotReady
	}

	err := checkObjectName(objectName)
	if err != nil {
		return nil, ErrObjectNameInvalid
	}

	objectElement := bu.objects.Find(MetaName(objectName))
	if objectElement != nil {
		obj := objectElement.(*object)
		return obj, nil
	}

	return nil, ErrObjectNotExist
}

func (l *LfsService) HeadObject(ctx context.Context, bucketName, objectName string) (*types.ObjectInfo, error) {
	ok := l.sw.TryAcquire(1)
	if !ok {
		return nil, ErrResourceUnavailable
	}
	defer l.sw.Release(1)

	bu, err := l.getBucketInfo(bucketName)
	if err != nil {
		return nil, err
	}

	object, err := l.getObjectInfo(bu, objectName)
	if err != nil {
		return nil, err
	}

	dist, donet, tt := 0, 0, 0
	for _, opID := range object.ops[1:] {
		total, dis, done := l.om.GetSegJogState(object.BucketID, opID)
		dist += dis
		donet += done
		tt += total
	}

	object.State = fmt.Sprintf("total %d, dispatch %d, done %d", tt, dist, donet)

	return &object.ObjectInfo, nil
}

func (l *LfsService) DeleteObject(ctx context.Context, bucketName, objectName string) (*types.ObjectInfo, error) {
	ok := l.sw.TryAcquire(1)
	if !ok {
		return nil, ErrResourceUnavailable
	}
	defer l.sw.Release(1)

	if !l.Writeable() {
		return nil, ErrLfsReadOnly
	}

	bucket, err := l.getBucketInfo(bucketName)
	if err != nil {
		return nil, err
	}

	object, err := l.getObjectInfo(bucket, objectName)
	if err != nil {
		return nil, err
	}

	bucket.Lock()
	defer bucket.Unlock()

	object.Lock()
	defer object.Unlock()

	deleteObject := pb.ObjectDeleteInfo{
		ObjectID: object.ObjectID,
		Time:     time.Now().Unix(),
	}

	payload, err := proto.Marshal(&deleteObject)
	if err != nil {
		return nil, err
	}

	op := &pb.OpRecord{
		Type:    pb.OpRecord_DeleteObject,
		Payload: payload,
	}

	err = bucket.addOpRecord(l.userID, op, l.ds)
	if err != nil {
		return nil, err
	}

	object.deletion = true
	object.dirty = true
	err = object.Save(l.userID, l.ds)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func (l *LfsService) ListObjects(ctx context.Context, bucketName string, opts *types.ListObjectsOptions) ([]*types.ObjectInfo, error) {
	ok := l.sw.TryAcquire(2)
	if !ok {
		return nil, ErrResourceUnavailable
	}
	defer l.sw.Release(2) //只读不需要Online

	if !l.Ready() {
		return nil, ErrLfsServiceNotReady
	}

	bucket, err := l.getBucketInfo(bucketName)
	if err != nil {
		return nil, err
	}

	bucket.RLock()
	defer bucket.RUnlock()

	var objects []*types.ObjectInfo
	objectIter := bucket.objects.Iterator()
	for objectIter != nil {
		object := objectIter.Value.(*object)
		if !object.deletion {
			if strings.HasPrefix(object.GetName(), opts.Prefix) {
				objects = append(objects, &object.ObjectInfo)
			}
		}
		objectIter = objectIter.Next()
	}

	return objects, nil
}

func xor(a []byte, b []byte) ([]byte, error) {
	if len(a) != len(b) {
		return nil, ErrWrongParameters
	}

	res := make([]byte, len(a))

	for i := 0; i < len(a); i++ {
		res[i] = a[i] ^ b[i]
	}
	return res, nil
}
