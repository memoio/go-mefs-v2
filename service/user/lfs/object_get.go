package lfs

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/shirou/gopsutil/v3/mem"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/lib/types"
)

// read at most one stripe
func (l *LfsService) GetObject(ctx context.Context, bucketName, objectName string, opts types.DownloadObjectOptions) ([]byte, error) {
	ok := l.sw.TryAcquire(2)
	if !ok {
		return nil, ErrResourceUnavailable
	}
	defer l.sw.Release(2)

	// 512MB?
	if opts.Length > 512*1024*1024 {
		v, err := mem.VirtualMemory()
		if err != nil {
			return nil, xerrors.Errorf("size is too large, consume too much memory")
		}
		if v.Available*10 < uint64(opts.Length)*12 {
			return nil, xerrors.Errorf("size is too large, memory is not enough")
		}
	}

	buf := new(bytes.Buffer)
	if bucketName == "" && objectName != "" {
		err := l.getObjectByCID(ctx, objectName, buf, opts)
		if err != nil {
			return nil, xerrors.Errorf("object %s download fail %s", objectName, err)
		}
	} else {
		err := l.getObject(ctx, bucketName, objectName, buf, opts)
		if err != nil {
			return nil, xerrors.Errorf("object %s download fail %s", objectName, err)
		}
	}

	return buf.Bytes(), nil
}

func (l *LfsService) downloadObject(ctx context.Context, bucket *bucket, object *object, writer io.Writer, opts types.DownloadObjectOptions) error {
	object.RLock()
	defer object.RUnlock()

	if object.deletion {
		return xerrors.Errorf("object %s is deleted", object.Name)
	}

	if object.Size == 0 {
		return xerrors.New("object is empty")
	}

	readStart := opts.Start
	readLength := opts.Length

	if readStart > int64(object.Size) ||
		readStart+readLength > int64(object.Size) {
		return xerrors.Errorf("out of object size %d", object.Size)
	}

	dp, ok := l.dps[bucket.BucketID]
	if !ok {
		ndp, err := l.newDataProcess(bucket.BucketID, &bucket.BucketOption)
		if err != nil {
			return err
		}
		dp = ndp
	}

	if readLength < 0 {
		readLength = int64(object.Size - uint64(readStart))
	}

	// length is zero
	if readLength == 0 {
		return xerrors.Errorf("read length is zero")
	}

	// read from each part
	accLen := uint64(0) // sum of part length
	rLen := uint64(0)   // have read ok
	for _, part := range object.Parts {
		logger.Debug("part: ", readStart, readLength, part.Offset, part.StoredBytes, part.Length)

		// forward to part
		if accLen+part.Length <= uint64(readStart) {
			accLen += part.Length
			continue
		}

		partStart := part.Offset
		partLength := part.Length

		if uint64(readStart) > accLen {
			// move forward
			partStart += (uint64(readStart) - accLen)
			// sub head
			partLength -= (uint64(readStart) - accLen)
		}

		if uint64(readStart+readLength) < accLen+part.Length {
			// sub end
			partLength -= (accLen + part.Length - uint64(readStart+readLength))
		}

		err := l.download(ctx, dp, bucket, object, int(partStart), int(partLength), writer)
		if err != nil {
			return err
		}
		rLen += partLength
		accLen += part.Length

		// read finish
		if rLen >= uint64(readLength) {
			break
		}

		// read to end
		if accLen >= uint64(readStart+readLength) {
			break
		}
	}
	return nil
}

func (l *LfsService) getObject(ctx context.Context, bucketName, objectName string, writer io.Writer, opts types.DownloadObjectOptions) error {
	bucket, err := l.getBucketInfo(bucketName)
	if err != nil {
		return err
	}

	if bucket.BucketInfo.Deletion {
		return xerrors.Errorf("bucket %d is deleted", bucket.BucketID)
	}

	object, err := l.getObjectInfo(bucket, objectName)
	if err != nil {
		return err
	}

	err = l.downloadObject(ctx, bucket, object, writer, opts)
	if err != nil {
		return xerrors.Errorf("object %s download fail %s", object.Name, err)
	}

	return nil
}

func (l *LfsService) getObjectByCID(ctx context.Context, cidName string, writer io.Writer, opts types.DownloadObjectOptions) error {
	od, ok := l.sb.cids[cidName]
	if !ok {
		return xerrors.Errorf("file not exist")
	}

	if len(l.sb.buckets) < int(od.bucketID) {
		return xerrors.Errorf("bucket %d not exist", od.bucketID)
	}

	bucket := l.sb.buckets[od.bucketID]
	if bucket.BucketInfo.Deletion {
		return xerrors.Errorf("bucket %d is deleted", od.bucketID)
	}

	object, ok := bucket.objects[od.objectID]
	if !ok {
		return xerrors.Errorf("object %d not exist", od.objectID)
	}

	return l.downloadObject(ctx, bucket, object, writer, opts)
}

func (l *LfsService) GetFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	bucketName := vars["bn"]
	objectName := vars["on"]

	start, err := strconv.ParseInt(vars["st"], 10, 64)
	if err != nil {
		start = 0
	}

	length, err := strconv.ParseInt(vars["le"], 10, 64)
	if err != nil {
		length = -1
	}

	if length == 0 {
		length = -1
	}

	logger.Debug("getfile : ", bucketName, objectName, start, length)

	doo := types.DownloadObjectOptions{
		Start:  start,
		Length: length,
	}

	//w.Header().Set("Content-Type", "application/octet-stream")
	err = l.getObject(r.Context(), bucketName, objectName, w, doo)
	if err != nil {
		w.WriteHeader(500)
		return
	}
}

func (l *LfsService) GetFileByCID(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cid := vars["cid"]
	start, err := strconv.ParseInt(vars["st"], 10, 64)
	if err != nil {
		start = 0
	}

	length, err := strconv.ParseInt(vars["le"], 10, 64)
	if err != nil {
		length = -1
	}

	if length == 0 {
		length = -1
	}

	logger.Debug("getfile : ", cid, start, length)
	doo := types.DownloadObjectOptions{
		Start:  start,
		Length: length,
	}
	//w.Header().Set("Content-Type", "application/octet-stream")
	err = l.getObjectByCID(r.Context(), cid, w, doo)
	if err != nil {
		w.WriteHeader(500)
		return
	}

}

func (l *LfsService) GetState(w http.ResponseWriter, r *http.Request) {

	gi, err := l.LfsGetInfo(r.Context(), false)
	if err != nil {
		w.WriteHeader(500)
	}

	json.NewEncoder(w).Encode(&gi)
}
