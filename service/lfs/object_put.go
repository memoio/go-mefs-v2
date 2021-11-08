package lfs

import (
	"context"
	"io"
	"log"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
)

func (l *LfsService) PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, opts types.PutObjectOptions) (*types.ObjectInfo, error) {
	ok := l.sw.TryAcquire(10)
	if !ok {
		return nil, ErrResourceUnavailable
	}
	defer l.sw.Release(10)

	logger.Infof("Upload object: %s to bucket: %s begin", objectName, bucketName)

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

	// 锁住这个object
	object.Lock()
	defer object.Unlock()

	// update objectInfo in bucket and metadata
	err = l.insertObject(ctx, bucket, object)
	if err != nil {
		return &object.ObjectInfo, err
	}

	part, err := l.createPart(ctx, bucket, object)
	if err != nil {
		return &object.ObjectInfo, err
	}

	err = l.addPartData(ctx, bucket, object, part, reader)
	if err != nil {
		return &object.ObjectInfo, err
	}

	//flush object meta and update bucket root
	err = l.appendPart(ctx, bucket, object, part)
	if err != nil {
		return &object.ObjectInfo, err
	}

	return &object.ObjectInfo, nil
}

func (l *LfsService) createObject(ctx context.Context, bucket *bucket, objectName string, opts types.PutObjectOptions) (*object, error) {
	objectElement := bucket.objects.Find(MetaName(objectName))
	if objectElement != nil {
		return nil, ErrObjectAlreadyExist
	}

	poi := pb.ObjectInfo{
		ObjectID:   bucket.NextObjectID,
		BucketID:   bucket.BucketID,
		Time:       time.Now().Unix(),
		Name:       objectName,
		Encryption: "aes",
	}

	// object 实例
	object := &object{
		ObjectInfo: types.ObjectInfo{
			ObjectInfo: poi,
		},
	}

	return object, nil
}

func (l *LfsService) createPart(ctx context.Context, bucket *bucket, object *object) (*pb.ObjectPartInfo, error) {
	return &pb.ObjectPartInfo{
		ObjectID:  object.GetObjectID(),
		UsedBytes: 0,
	}, nil
}

func (l *LfsService) addPartData(ctx context.Context, bucket *bucket, object *object, part *pb.ObjectPartInfo, reader io.Reader) error {

	return nil
}

func (l *LfsService) insertObject(ctx context.Context, bucket *bucket, object *object) error {
	// 将本次操作序列化存储
	payload, err := proto.Marshal(&object.ObjectInfo)
	if err != nil {
		return err
	}
	// 生成Operation
	op := &pb.OpRecord{
		Type:    pb.OpRecord_CreateObject,
		OpID:    bucket.NextOpID,
		Payload: payload,
	}

	// 存储操作信息
	err = saveOpRecord(l.fsID, bucket.BucketID, op, l.ds)
	if err != nil {
		return err
	}

	// 操作数++
	bucket.BucketInfo.NextOpID++

	// 存储object信息
	err = object.Save(l.fsID, bucket.BucketID, l.ds)
	if err != nil {
		return err
	}

	// 将对象插入红黑树
	bucket.objects.Insert(MetaName(object.GetName()), object)

	// 对象数++
	bucket.NextObjectID++

	//gen_root
	tag, err := proto.Marshal(op)
	if err != nil {
		return err
	}
	bucket.mtree.Push(tag)
	bucket.Root = bucket.mtree.Root()
	bucket.dirty = true
	bucket.MTime = time.Now().Unix()

	log.Printf("Upload create object: %s in bucket: %s", object.GetName(), bucket.GetName())
	return nil
}

func (l *LfsService) appendPart(ctx context.Context, bucket *bucket, object *object, part *pb.ObjectPartInfo) error {
	object.Parts = append(object.Parts, part)

	// bucket
	bucket.MTime = part.Time
	payload, err := proto.Marshal(part)
	if err != nil {
		return err
	}
	op := &pb.OpRecord{
		Type:    pb.OpRecord_AddData,
		OpID:    bucket.NextOpID,
		Payload: payload,
	}

	err = saveOpRecord(l.fsID, bucket.BucketID, op, l.ds)
	if err != nil {
		return err
	}
	bucket.NextOpID++

	// 存储object信息
	err = object.Save(l.fsID, bucket.BucketID, l.ds)
	if err != nil {
		return err
	}

	// leaf is OpID + PayLoad
	tag, err := proto.Marshal(op)
	if err != nil {
		return err
	}
	bucket.mtree.Push(tag)
	bucket.Root = bucket.mtree.Root()
	l.sb.dirty = true

	bucket.dirty = true
	log.Printf("Add data to object: %s in bucket: %s end, length is: %d", object.GetName(), bucket.GetName(), part.Length)
	return nil
}

func calculateETagForNewPart(old, new []byte) []byte {
	oldBytes := make([]byte, len(old))
	newBytes := make([]byte, len(new))
	copy(oldBytes, old)
	copy(newBytes, new)
	var err error
	if len(oldBytes) == 0 {
		return newBytes
	}

	if len(newBytes) == 0 {
		return oldBytes
	}

	err = xor(oldBytes, newBytes)
	if err != nil {
		return nil
	}

	return oldBytes
}

func xor(a []byte, b []byte) error {
	if len(a) != len(b) {
		return ErrWrongParameters
	}
	for i := 0; i < len(a); i++ {
		a[i] ^= b[i]
	}
	return nil
}
