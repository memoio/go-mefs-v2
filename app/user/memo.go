package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	minio "github.com/memoio/minio/cmd"
	"golang.org/x/xerrors"

	mclient "github.com/memoio/go-mefs-v2/api/client"
	"github.com/memoio/go-mefs-v2/app/user/mutipart"
	"github.com/memoio/go-mefs-v2/build"
	mcode "github.com/memoio/go-mefs-v2/lib/code"
	"github.com/memoio/go-mefs-v2/lib/pb"
	mtypes "github.com/memoio/go-mefs-v2/lib/types"
	metag "github.com/memoio/go-mefs-v2/lib/utils/etag"
)

var (
	minioMetaBucket      = ".minio.sys"
	dataUsageObjNamePath = "buckets/.usage.json"
	DefaultBufSize       = 1024 * 1024 * 4
)

var (
	errLfsServiceNotReady = xerrors.New("lfs service not ready")
	errBucket             = xerrors.New("bucket error")
	errObject             = xerrors.New("object error")
	errMutiUpload         = xerrors.New("mutiupload error")
	errAbort              = xerrors.New("MultipartUpload abort")
)

type MemoFs struct {
	addr    string
	headers http.Header
}

func (l *lfsGateway) getMemofs() error {
	var err error
	l.memofs, err = NewMemofs()
	if err != nil {
		return err
	}
	return nil
}

func NewMemofs() (*MemoFs, error) {
	repoDir := os.Getenv("MEFS_PATH")
	addr, headers, err := mclient.GetMemoClientInfo(repoDir)
	if err != nil {
		return nil, err
	}
	napi, closer, err := mclient.NewUserNode(context.Background(), addr, headers)
	if err != nil {
		return nil, err
	}
	defer closer()
	_, err = napi.ShowStorage(context.Background())
	if err != nil {
		return nil, err
	}

	return &MemoFs{
		addr:    addr,
		headers: headers,
	}, nil
}

func (m *MemoFs) MakeBucketWithLocation(ctx context.Context, bucket string) error {
	napi, closer, err := mclient.NewUserNode(ctx, m.addr, m.headers)
	if err != nil {
		return convertToMinioError(errLfsServiceNotReady, bucket, "")
	}
	defer closer()
	if bucket == "" {
		return convertToMinioError(errBucket, bucket, "")
	}
	opts := mcode.DefaultBucketOptions()

	_, err = napi.CreateBucket(ctx, bucket, opts)
	if err != nil {
		return convertToMinioError(errBucket, bucket, "")
	}
	return nil
}

func (m *MemoFs) GetBucketInfo(ctx context.Context, bucket string) (bi mtypes.BucketInfo, err error) {
	napi, closer, err := mclient.NewUserNode(ctx, m.addr, m.headers)
	if err != nil {
		return bi, convertToMinioError(errLfsServiceNotReady, bucket, "")
	}
	defer closer()

	bi, err = napi.HeadBucket(ctx, bucket)
	if err != nil {
		return bi, convertToMinioError(errBucket, bucket, "")
	}
	return bi, nil
}

func (m *MemoFs) ListBuckets(ctx context.Context) ([]mtypes.BucketInfo, error) {
	napi, closer, err := mclient.NewUserNode(ctx, m.addr, m.headers)
	if err != nil {
		return nil, convertToMinioError(errLfsServiceNotReady, "", "")
	}
	defer closer()

	buckets, err := napi.ListBuckets(ctx, "")
	if err != nil {
		return nil, convertToMinioError(errBucket, "", "")
	}
	return buckets, nil
}

func (m *MemoFs) ListObjects(ctx context.Context, bucket string, prefix, marker, delimiter string, maxKeys int) (mloi mtypes.ListObjectsInfo, err error) {
	napi, closer, err := mclient.NewUserNode(ctx, m.addr, m.headers)
	if err != nil {
		return mloi, convertToMinioError(errLfsServiceNotReady, bucket, "")
	}
	defer closer()
	loo := mtypes.ListObjectsOptions{
		Prefix:    prefix,
		Marker:    marker,
		Delimiter: delimiter,
		MaxKeys:   maxKeys,
	}

	mloi, err = napi.ListObjects(ctx, bucket, loo)
	if err != nil {
		return mloi, convertToMinioError(errObject, bucket, "")
	}
	return mloi, nil
}

func (m *MemoFs) GetObject(ctx context.Context, bucketName, objectName string, startOffset, length int64, writer io.Writer) error {
	if bucketName == minioMetaBucket && objectName == dataUsageObjNamePath {
		mtime := int64(0)
		dui := minio.DataUsageInfo{
			BucketsUsage: make(map[string]minio.BucketUsageInfo),
		}

		bus, err := m.ListBuckets(ctx)
		if err != nil {
			return convertToMinioError(errObject, bucketName, objectName)
		}
		for _, bu := range bus {
			if mtime > bu.MTime {
				mtime = bu.MTime
			}
			bui := minio.BucketUsageInfo{
				Size:         bu.Length,
				ObjectsCount: bu.NextObjectID,
				ReplicaSize:  bu.UsedBytes,
			}
			dui.BucketsUsage[bu.Name] = bui
			dui.ObjectsTotalSize += bui.Size
			dui.ObjectsTotalCount += bui.ObjectsCount
			dui.BucketsCount++
		}

		dui.LastUpdate = time.Unix(mtime, 0)

		res, err := json.Marshal(dui)
		if err != nil {
			return convertToMinioError(errObject, bucketName, objectName)
		}
		writer.Write(res)
		return nil
	}

	napi, closer, err := mclient.NewUserNode(ctx, m.addr, m.headers)
	if err != nil {
		return convertToMinioError(errLfsServiceNotReady, bucketName, objectName)
	}
	defer closer()

	objInfo, err := napi.HeadObject(ctx, bucketName, objectName)
	if err != nil {
		return convertToMinioError(errObject, bucketName, objectName)
	}

	if length == -1 {
		length = int64(objInfo.Size)
	}

	buInfo, err := napi.HeadBucket(ctx, bucketName)
	if err != nil {
		return convertToMinioError(errObject, bucketName, objectName)
	}

	stepLen := int64(build.DefaultSegSize * buInfo.DataCount)
	stepAccMax := 512 / int(buInfo.DataCount) // 128MB

	start := int64(startOffset)
	end := startOffset + length
	stepacc := 1
	for start < end {
		if stepacc > stepAccMax {
			stepacc = stepAccMax
		}

		readLen := stepLen*int64(stepacc) - (start % stepLen)
		if end-start < readLen {
			readLen = end - start
		}

		doo := mtypes.DownloadObjectOptions{
			Start:  start,
			Length: readLen,
		}

		data, err := napi.GetObject(ctx, bucketName, objectName, doo)
		if err != nil {
			//log.Println("received length err is:", start, readLen, stepLen, err)
			break
		}

		writer.Write(data)
		start += int64(readLen)
		stepacc *= 2
	}

	return nil
}

func (m *MemoFs) GetObjectInfo(ctx context.Context, bucket, object string) (objInfo mtypes.ObjectInfo, err error) {
	if bucket == minioMetaBucket && object == dataUsageObjNamePath {
		return mtypes.ObjectInfo{
			ObjectInfo: pb.ObjectInfo{
				Time: time.Now().Unix(),
				Name: dataUsageObjNamePath,
			},
			Size: 4 * 1024,
		}, nil
	}

	napi, closer, err := mclient.NewUserNode(ctx, m.addr, m.headers)
	if err != nil {
		return objInfo, convertToMinioError(errLfsServiceNotReady, bucket, object)
	}
	defer closer()

	oi, err := napi.HeadObject(ctx, bucket, object)
	if err != nil {
		return objInfo, convertToMinioError(errObject, bucket, object)
	}
	return oi, nil
}

func (m *MemoFs) PutObject(ctx context.Context, bucket, object string, r io.Reader, UserDefined map[string]string) (objInfo mtypes.ObjectInfo, err error) {
	napi, closer, err := mclient.NewUserNode(ctx, m.addr, m.headers)
	if err != nil {
		return objInfo, convertToMinioError(errLfsServiceNotReady, bucket, object)
	}
	defer closer()

	poo := mtypes.DefaultUploadOption()
	for k, v := range UserDefined {
		poo.UserDefined[k] = v
	}
	moi, err := napi.PutObject(ctx, bucket, object, r, poo)
	if err != nil {
		return objInfo, convertToMinioError(errObject, bucket, object)
	}
	return moi, nil
}

func (m *MemoFs) StatObject(ctx context.Context, bucket, object string) (objInfo mtypes.ObjectInfo, err error) {
	napi, closer, err := mclient.NewUserNode(ctx, m.addr, m.headers)
	if err != nil {
		return objInfo, convertToMinioError(errLfsServiceNotReady, bucket, object)
	}
	defer closer()

	oi, err := napi.HeadObject(ctx, bucket, object)
	if err != nil {
		return objInfo, convertToMinioError(errObject, bucket, object)
	}
	return oi, nil
}

func (m *MemoFs) DeleteObject(ctx context.Context, bucket, object string) error {
	napi, closer, err := mclient.NewUserNode(ctx, m.addr, m.headers)
	if err != nil {
		return convertToMinioError(errLfsServiceNotReady, bucket, object)
	}
	defer closer()
	err = napi.DeleteObject(ctx, bucket, object)
	if err != nil {
		return convertToMinioError(errObject, bucket, object)
	}
	return nil
}

func (m *MemoFs) NewMultipartUpload(ctx context.Context, bucket, object string, metadata map[string]string, uploads *mutipart.MultipartUploads) (uploadID string, err error) {
	ctx, cancel := context.WithCancel(context.Background())
	// get bucket with bucket name
	_, err = m.GetBucketInfo(ctx, bucket)
	if err != nil {
		cancel()
		return uploadID, err
	}

	upload, err := uploads.Create(bucket, object, cancel)
	if err != nil {
		return uploadID, convertToMinioError(errMutiUpload, bucket, object)
	}

	go func() {
		reader := bufio.NewReaderSize(upload.Stream, DefaultBufSize)
		moi, err := m.PutObject(ctx, bucket, object, reader, metadata)
		uploads.RemoveByID(upload.ID)
		if err != nil {
			upload.Fail(err)
		} else {
			etag, _ := metag.ToString(moi.ETag)
			upload.Complete(minio.ObjectInfo{
				Bucket:  bucket,
				Name:    object,
				ModTime: time.Unix(moi.GetTime(), 0),
				Size:    int64(moi.Size),
				ETag:    etag,
				IsDir:   false,
			})
		}
	}()

	return upload.ID, nil
}

func (m *MemoFs) PutObjectPart(ctx context.Context, bucket, object, uploadID string, partID int, data *minio.PutObjReader, opts minio.ObjectOptions, uploads *mutipart.MultipartUploads) (minio.PartInfo, error) {
	upload, err := uploads.Get(bucket, object, uploadID)
	if err != nil {
		return minio.PartInfo{}, err
	}
	part, err := upload.Stream.AddPart(partID, data.Reader)
	if err != nil {
		return minio.PartInfo{}, err
	}

	err = <-part.Done
	if err != nil {
		return minio.PartInfo{}, err
	}

	partInfo := minio.PartInfo{
		PartNumber:   part.Number,
		LastModified: time.Now(),
		ETag:         data.SHA256HexString(),
		Size:         atomic.LoadInt64(&part.Size),
	}
	upload.AddCompletedPart(partInfo)
	return partInfo, nil
}

func convertToMinioError(err error, bucket, object string) error {
	switch err {
	case errLfsServiceNotReady:
		return minio.BackendDown{}
	case errBucket:
		if strings.Contains(err.Error(), "invalid") {
			return minio.BucketNameInvalid{Bucket: bucket}
		}
		if strings.Contains(err.Error(), "not exist") {
			return minio.BucketNotFound{Bucket: bucket}
		}
		if strings.Contains(err.Error(), "already exists") {
			return minio.BucketAlreadyExists{Bucket: bucket}
		}
		return minio.PrefixAccessDenied{Bucket: bucket, Object: object}
	case errObject:
		if strings.Contains(err.Error(), "invalid") {
			return minio.ObjectNameInvalid{Bucket: bucket, Object: object}
		}
		if strings.Contains(err.Error(), "not exist") {
			return minio.ObjectNotFound{Bucket: bucket, Object: object}
		}
		if strings.Contains(err.Error(), "already exists") {
			return minio.ObjectAlreadyExists{Bucket: bucket, Object: object}
		}
		return minio.PrefixAccessDenied{Bucket: bucket, Object: object}
	case errMutiUpload:
		return minio.PrefixAccessDenied{Bucket: bucket, Object: object}
	case nil:
		return nil
	default:
		return minio.PrefixAccessDenied{Bucket: bucket, Object: object}

	}
}
