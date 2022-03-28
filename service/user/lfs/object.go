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

	objectElement := bu.objectTree.Find(MetaName(objectName))
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
	for _, opID := range object.ops[1 : 1+len(object.Parts)] {
		total, dis, done, c := l.OrderMgr.GetSegJogState(object.BucketID, opID)
		dist += dis
		donet += done
		tt += total
		ct += c
	}

	object.State = fmt.Sprintf("total %d, dispatch %d, sent %d, confirm %d", tt, dist, donet, ct)

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

	if bucket.BucketID >= l.sb.bucketVerify {
		return nil, xerrors.Errorf("bucket %d is confirming", bucket.BucketID)
	}

	bucket.Lock()
	defer bucket.Unlock()

	object, err := l.getObjectInfo(bucket, objectName)
	if err != nil {
		return nil, err
	}

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

	bucket.objectTree.Delete(MetaName(objectName))
	delete(bucket.objects, object.ObjectID)

	return nil, nil
}

func (l *LfsService) ListObjects(ctx context.Context, bucketName string, opts *types.ListObjectsOptions) (types.ListObjectsInfo, error) {
	res := types.ListObjectsInfo{}
	ok := l.sw.TryAcquire(2)
	if !ok {
		return res, ErrResourceUnavailable
	}
	defer l.sw.Release(2) //只读不需要Online

	if opts.Delimiter != "" && opts.Delimiter != SlashSeparator {
		return res, xerrors.Errorf("unsupported delimeter %s", opts.Delimiter)
	}

	if opts.Delimiter == SlashSeparator && opts.Prefix == SlashSeparator {
		return res, nil
	}

	if opts.Marker != "" {
		// not have preifx
		if !strings.HasPrefix(opts.Marker, opts.Prefix) {
			return res, nil
		}
	}

	if opts.MaxKeys == 0 {
		return res, nil
	}

	if opts.MaxKeys < 0 || opts.MaxKeys > types.MaxListKeys {
		opts.MaxKeys = types.MaxListKeys
	}

	bucket, err := l.getBucketInfo(bucketName)
	if err != nil {
		return res, err
	}

	bucket.RLock()
	defer bucket.RUnlock()

	if opts.MaxKeys > bucket.objectTree.Size() {
		opts.MaxKeys = bucket.objectTree.Size()
	}

	entryPrefixMatch := opts.Prefix
	if opts.Delimiter != "" {
		lastIndex := strings.LastIndex(opts.Prefix, opts.Delimiter)
		if lastIndex > 0 && lastIndex < len(opts.Prefix) {
			entryPrefixMatch = opts.Prefix[:lastIndex+1]
		}
	}
	prefixLen := len(entryPrefixMatch)

	preMap := make(map[string]struct{})
	res.Objects = make([]types.ObjectInfo, 0, opts.MaxKeys)
	cnt := 0
	if !bucket.objectTree.Empty() {
		objectIter := bucket.objectTree.Iterator()
		if opts.Marker != "" {
			objectIter = bucket.objectTree.FindIt(MetaName(opts.Marker))
		}
		for objectIter != nil {
			object, ok := objectIter.Value.(*object)
			if ok && !object.deletion {
				on := object.GetName()
				res.NextMarker = on
				if strings.HasPrefix(on, opts.Prefix) {
					ind := strings.Index(on[prefixLen:], opts.Delimiter)
					if ind >= 0 {
						npr := on[:prefixLen+ind+1]
						_, has := preMap[npr]
						if !has {
							// add to common prefix
							res.Prefixes = append(res.Prefixes, on[:prefixLen+ind+1])
							preMap[npr] = struct{}{}
							cnt++
						}
					} else {
						res.Objects = append(res.Objects, object.ObjectInfo)
						cnt++
					}
				}
			}
			if cnt >= opts.MaxKeys {
				break
			}
			objectIter = objectIter.Next()
		}

		if objectIter != nil {
			res.IsTruncated = true
		}
	}

	return res, nil
}
