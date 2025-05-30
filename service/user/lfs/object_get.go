package lfs

import (
	"bytes"
	"context"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/shirou/gopsutil/v3/mem"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/lib/crypto/pdp"
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
		err := l.getObjectByEtag(ctx, objectName, buf, opts)
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

func (l *LfsService) downloadObject(ctx context.Context, userID uint64, bi types.BucketInfo, object *object, writer io.Writer, opts types.DownloadObjectOptions) error {
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

	dp, err := l.getDataProcess(ctx, userID, bi.BucketID, &bi.BucketOption)
	if err != nil {
		return err
	}

	if userID != l.userID {
		if opts.UserDefined != nil && opts.UserDefined["decrypt"] != "" {
			dec, err := hex.DecodeString(opts.UserDefined["decrypt"])
			if err == nil {
				copy(dp.aesKey[:], dec)
			}
		}
	}

	dv, err := pdp.NewDataVerifier(dp.keyset.PublicKey(), dp.keyset.SecreteKey())
	if err != nil {
		return err
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

		err := l.download(ctx, dp, dv, bi, object, int(partStart), int(partLength), writer)
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

	err = l.downloadObject(ctx, l.userID, bucket.BucketInfo, object, writer, opts)
	if err != nil {
		return xerrors.Errorf("object %s download fail %s", object.Name, err)
	}

	return nil
}

func (l *LfsService) getObjectByEtag(ctx context.Context, etagName string, writer io.Writer, opts types.DownloadObjectOptions) error {
	odi, ok := l.sb.etagCache.Get(etagName)
	if !ok {
		return l.downloadOtherObjectByEtag(ctx, etagName, writer, opts)
	}

	od, ok := odi.(*objectDigest)
	if !ok {
		return xerrors.Errorf("wrong type in etag cache")
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

	return l.downloadObject(ctx, l.userID, bucket.BucketInfo, object, writer, opts)
}

func (l *LfsService) DownloadFile(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		w.WriteHeader(500)
		return
	}

	etagName := r.Form.Get("etag")

	start, err := strconv.ParseInt(r.Form.Get("start"), 10, 64)
	if err != nil {
		start = 0
	}

	length, err := strconv.ParseInt(r.Form.Get("length"), 10, 64)
	if err != nil {
		length = -1
	}

	if length == 0 {
		length = -1
	}

	doo := types.DownloadObjectOptions{
		Start:  start,
		Length: length,
	}

	if etagName != "" {
		logger.Debug("getfile : ", etagName, start, length)
		err = l.getObjectByEtag(r.Context(), etagName, w, doo)
		if err != nil {
			w.WriteHeader(500)
			return
		}
		return
	}

	bucketName := r.Form.Get("bucket")
	objectName := r.Form.Get("object")

	logger.Debug("getfile : ", bucketName, objectName, start, length)

	//w.Header().Set("Content-Type", "application/octet-stream")
	err = l.getObject(r.Context(), bucketName, objectName, w, doo)
	if err != nil {
		w.WriteHeader(500)
		return
	}
}

func (l *LfsService) GetFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	etagName := vars["etag"]

	doo := types.DownloadObjectOptions{
		Start:  0,
		Length: -1,
	}

	if etagName != "" {
		logger.Debug("getfile : ", etagName)
		err := l.getObjectByEtag(r.Context(), etagName, w, doo)
		if err != nil {
			w.WriteHeader(500)
			return
		}
		return
	}

	bucketName := vars["bucket"]
	objectName := vars["object"]

	logger.Debug("getfile : ", bucketName, objectName)

	err := l.getObject(r.Context(), bucketName, objectName, w, doo)
	if err != nil {
		w.WriteHeader(500)
		return
	}
}
