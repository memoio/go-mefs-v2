package lfs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"golang.org/x/xerrors"
)

func (l *LfsService) getObjectInfo(bu *bucket, objectName string) (*object, error) {
	err := checkObjectName(objectName)
	if err != nil {
		return nil, xerrors.Errorf("object name is invalid: %s", err)
	}

	objectElement := bu.objects.Find(MetaName(objectName))
	if objectElement != nil {
		obj := objectElement.(*object)
		return obj, nil
	}

	return nil, xerrors.Errorf("object %s not exist", objectName)
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

	tt, dist, donet, ct := 0, 0, 0, 0
	for _, opID := range object.ops[1:] {
		total, dis, done, c := l.OrderMgr.GetSegJogState(object.BucketID, opID)
		dist += dis
		donet += done
		tt += total
		ct += c
	}

	object.State = fmt.Sprintf("total %d, dispatch %d, done %d, confirm %d", tt, dist, donet, ct)

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

	bucket.objects.Delete(MetaName(objectName))

	return nil, nil
}

func (l *LfsService) ListObjects(ctx context.Context, bucketName string, opts *types.ListObjectsOptions) ([]*types.ObjectInfo, error) {
	ok := l.sw.TryAcquire(2)
	if !ok {
		return nil, ErrResourceUnavailable
	}
	defer l.sw.Release(2) //只读不需要Online

	bucket, err := l.getBucketInfo(bucketName)
	if err != nil {
		return nil, err
	}

	bucket.RLock()
	defer bucket.RUnlock()

	var objects []*types.ObjectInfo

	if !bucket.objects.Empty() {
		objectIter := bucket.objects.Iterator()
		for objectIter != nil {
			object, ok := objectIter.Value.(*object)
			if ok && !object.deletion {
				if strings.HasPrefix(object.GetName(), opts.Prefix) {
					objects = append(objects, &object.ObjectInfo)
				}
			}
			objectIter = objectIter.Next()
		}
	}

	return objects, nil
}
