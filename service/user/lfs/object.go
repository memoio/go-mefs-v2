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

func (l *LfsService) HeadObject(ctx context.Context, bucketName, objectName string) (types.ObjectInfo, error) {
	oi := types.ObjectInfo{}
	ok := l.sw.TryAcquire(1)
	if !ok {
		return oi, ErrResourceUnavailable
	}
	defer l.sw.Release(1)

	if bucketName == "" {
		od, ok := l.sb.cids[objectName]
		if !ok {
			return oi, xerrors.Errorf("file not exist")
		}

		if len(l.sb.buckets) < int(od.bucketID) {
			return oi, xerrors.Errorf("bucket %d not exist", od.bucketID)
		}

		bucket := l.sb.buckets[od.bucketID]
		if bucket.BucketInfo.Deletion {
			return oi, xerrors.Errorf("bucket %d is deleted", od.bucketID)
		}

		object, ok := bucket.objects[od.objectID]
		if !ok {
			return oi, xerrors.Errorf("object %d not exist", od.objectID)
		}

		if !object.pin {
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
		}

		return object.ObjectInfo, nil
	}

	bu, err := l.getBucketInfo(bucketName)
	if err != nil {
		return oi, err
	}

	object, err := l.getObjectInfo(bu, objectName)
	if err != nil {
		return oi, err
	}

	if !object.pin {
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
	}

	return object.ObjectInfo, nil
}

func (l *LfsService) DeleteObject(ctx context.Context, bucketName, objectName string) error {
	ok := l.sw.TryAcquire(1)
	if !ok {
		return ErrResourceUnavailable
	}
	defer l.sw.Release(1)

	if !l.Writeable() {
		return ErrLfsReadOnly
	}

	bucket, err := l.getBucketInfo(bucketName)
	if err != nil {
		return err
	}

	if bucket.BucketID >= l.sb.bucketVerify {
		return xerrors.Errorf("bucket %d is confirming", bucket.BucketID)
	}

	bucket.Lock()
	defer bucket.Unlock()

	object, err := l.getObjectInfo(bucket, objectName)
	if err != nil {
		return err
	}

	object.Lock()
	defer object.Unlock()

	deleteObject := pb.ObjectDeleteInfo{
		ObjectID: object.ObjectID,
		Time:     time.Now().Unix(),
	}

	payload, err := proto.Marshal(&deleteObject)
	if err != nil {
		return err
	}

	op := &pb.OpRecord{
		Type:    pb.OpRecord_DeleteObject,
		Payload: payload,
	}

	err = bucket.addOpRecord(l.userID, op, l.ds)
	if err != nil {
		return err
	}

	object.ops = append(object.ops, op.OpID)
	object.deletion = true
	object.dirty = true
	err = object.Save(l.userID, l.ds)
	if err != nil {
		return err
	}

	bucket.objectTree.Delete(MetaName(objectName))
	delete(bucket.objects, object.ObjectID)

	return nil
}

func (l *LfsService) ListObjects(ctx context.Context, bucketName string, opts types.ListObjectsOptions) (types.ListObjectsInfo, error) {
	res := types.ListObjectsInfo{}
	ok := l.sw.TryAcquire(2)
	if !ok {
		return res, ErrResourceUnavailable
	}
	defer l.sw.Release(2) //只读不需要Online

	if opts.Delimiter != "" && opts.Delimiter != SlashSeparator {
		return res, xerrors.Errorf("unsupported delimiter: %s, only '/' or ''", opts.Delimiter)
	}

	// as key format is xx/yyy
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

	// remove read lock
	//bucket.RLock()
	//defer bucket.RUnlock()

	if opts.MaxKeys > bucket.objectTree.Size() {
		opts.MaxKeys = bucket.objectTree.Size()
	}

	recursive := true
	if opts.Delimiter == SlashSeparator {
		recursive = false
	}

	entryPrefixMatch := opts.Prefix
	if !recursive {
		lastIndex := strings.LastIndex(opts.Prefix, opts.Delimiter)
		if lastIndex > 0 && lastIndex < len(opts.Prefix) {
			entryPrefixMatch = opts.Prefix[:lastIndex+1]
		}
	}
	prefixLen := len(entryPrefixMatch)

	preMap := make(map[string]struct{})
	res.Objects = make([]types.ObjectInfo, 0, opts.MaxKeys)
	cnt := opts.MaxKeys
	if !bucket.objectTree.Empty() {
		objectIter := bucket.objectTree.Iterator()
		if opts.Marker != "" {
			objectIter = bucket.objectTree.FindIt(MetaName(opts.Marker))
		}
		for objectIter != nil {
			object, ok := objectIter.Value.(*object)
			if ok && !object.deletion {
				on := object.GetName()
				if strings.HasPrefix(on, opts.Prefix) {
					if recursive {
						res.Objects = append(res.Objects, object.ObjectInfo)
						cnt--
					} else {
						ind := strings.Index(on[prefixLen:], opts.Delimiter)
						if ind >= 0 {
							npr := on[:prefixLen+ind+1]
							_, has := preMap[npr]
							if !has {
								// add to common prefix
								res.Prefixes = append(res.Prefixes, on[:prefixLen+ind+1])
								preMap[npr] = struct{}{}
								cnt--
							}
						} else {
							res.Objects = append(res.Objects, object.ObjectInfo)
							cnt--
						}
					}
				}
			}
			if cnt <= 0 {
				break
			}
			objectIter = objectIter.Next()
		}

		// netx object is marker
		if objectIter != nil {
			objectIter = objectIter.Next()
			if objectIter != nil {
				res.IsTruncated = true
				object, ok := objectIter.Value.(*object)
				if ok {
					res.NextMarker = object.GetName()
				}
			}
		}
	}

	return res, nil
}
