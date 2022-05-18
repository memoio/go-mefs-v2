package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"

	minio "github.com/memoio/minio/cmd"
	"golang.org/x/xerrors"

	mclient "github.com/memoio/go-mefs-v2/api/client"
	"github.com/memoio/go-mefs-v2/build"
	mcode "github.com/memoio/go-mefs-v2/lib/code"
	"github.com/memoio/go-mefs-v2/lib/pb"
	mtypes "github.com/memoio/go-mefs-v2/lib/types"
)

var (
	minioMetaBucket      = ".minio.sys"
	dataUsageObjNamePath = "buckets/.usage.json"
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
		return err
	}
	defer closer()
	if bucket == "" {
		return xerrors.New("bucketname is nil")
	}
	opts := mcode.DefaultBucketOptions()

	_, err = napi.CreateBucket(ctx, bucket, opts)
	if err != nil {
		return err
	}
	return nil
}

func (m *MemoFs) GetBucketInfo(ctx context.Context, bucket string) (bi mtypes.BucketInfo, err error) {
	napi, closer, err := mclient.NewUserNode(ctx, m.addr, m.headers)
	if err != nil {
		return bi, err
	}
	defer closer()

	bi, err = napi.HeadBucket(ctx, bucket)
	if err != nil {
		return bi, err
	}
	return bi, nil
}

func (m *MemoFs) ListBuckets(ctx context.Context) ([]mtypes.BucketInfo, error) {
	napi, closer, err := mclient.NewUserNode(ctx, m.addr, m.headers)
	if err != nil {
		return nil, err
	}
	defer closer()

	buckets, err := napi.ListBuckets(ctx, "")
	if err != nil {
		return nil, err
	}
	return buckets, nil
}

func (m *MemoFs) ListObjects(ctx context.Context, bucket string, prefix, marker, delimiter string, maxKeys int) (mloi mtypes.ListObjectsInfo, err error) {
	napi, closer, err := mclient.NewUserNode(ctx, m.addr, m.headers)
	if err != nil {
		return mloi, err
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
		return mloi, err
	}
	return mloi, nil
}

func (m *MemoFs) GetObject(ctx context.Context, bucketName, objectName string, startOffset, length int64, writer io.Writer) error {
	// for minio console, simulate
	if bucketName == minioMetaBucket && objectName == dataUsageObjNamePath {
		mtime := int64(0)
		dui := minio.DataUsageInfo{
			BucketsUsage: make(map[string]minio.BucketUsageInfo),
		}

		bus, err := m.ListBuckets(ctx)
		if err != nil {
			return err
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
			return err
		}
		writer.Write(res)
		return nil
	}

	napi, closer, err := mclient.NewUserNode(ctx, m.addr, m.headers)
	if err != nil {
		return err
	}
	defer closer()

	objInfo, err := napi.HeadObject(ctx, bucketName, objectName)
	if err != nil {
		return err
	}

	if length == -1 {
		length = int64(objInfo.Size)
	}

	buInfo, err := napi.HeadBucket(ctx, bucketName)
	if err != nil {
		return err
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
		return objInfo, err
	}
	defer closer()

	oi, err := napi.HeadObject(ctx, bucket, object)
	if err != nil {
		return objInfo, err
	}
	return oi, nil
}

func (m *MemoFs) PutObject(ctx context.Context, bucket, object string, r io.Reader, UserDefined map[string]string) (objInfo mtypes.ObjectInfo, err error) {
	napi, closer, err := mclient.NewUserNode(ctx, m.addr, m.headers)
	if err != nil {
		return objInfo, err
	}
	defer closer()

	poo := mtypes.DefaultUploadOption()
	for k, v := range UserDefined {
		poo.UserDefined[k] = v
	}
	moi, err := napi.PutObject(ctx, bucket, object, r, poo)
	if err != nil {
		return objInfo, err
	}
	return moi, nil
}
