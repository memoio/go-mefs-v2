package lfs

import (
	"context"
	"crypto/md5"
	"encoding/binary"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/zeebo/blake3"
	"golang.org/x/sync/semaphore"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/code"
	"github.com/memoio/go-mefs-v2/lib/crypto/aes"
	"github.com/memoio/go-mefs-v2/lib/crypto/pdp"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/lib/utils/etag"
)

type dataProcess struct {
	sync.Mutex

	bucketID    uint64
	dataCount   int
	parityCount int
	stripeSize  int // dataCount*segSize

	aesKey [32]byte          // aes encrypt base key
	segID  segment.SegmentID // for encode
	coder  code.Codec

	dv pdpcommon.DataVerifier
}

func (l *LfsService) newDataProcess(bucketID uint64, bopt *pb.BucketOption) (*dataProcess, error) {
	coder, err := code.NewDataCoderWithBopts(l.keyset, bopt)
	if err != nil {
		return nil, err
	}

	tmpkey := make([]byte, len(l.encryptKey)+8)
	copy(tmpkey, l.encryptKey)
	binary.BigEndian.PutUint64(tmpkey[len(l.encryptKey):], bucketID)
	hres := blake3.Sum256(tmpkey)

	stripeSize := int(bopt.SegSize) * int(bopt.DataCount)

	segID, err := segment.NewSegmentID(l.fsID, bucketID, 0, 0)
	if err != nil {
		return nil, err
	}

	dv, err := pdp.NewDataVerifier(l.keyset.PublicKey(), l.keyset.SecreteKey())
	if err != nil {
		return nil, err
	}

	dp := &dataProcess{
		bucketID:    bucketID,
		dataCount:   int(bopt.DataCount),
		parityCount: int(bopt.ParityCount),
		stripeSize:  stripeSize,

		aesKey: hres,
		coder:  coder,
		segID:  segID,

		dv: dv,
	}

	l.Lock()
	l.dps[bucketID] = dp
	l.Unlock()
	return dp, nil
}

func (l *LfsService) upload(ctx context.Context, bucket *bucket, object *object, r io.Reader, opts types.PutObjectOptions) error {
	nt := time.Now()
	logger.Debug("upload begin at: ", nt)
	l.RLock()
	dp, ok := l.dps[bucket.BucketID]
	l.RUnlock()
	if !ok {
		ndp, err := l.newDataProcess(bucket.BucketID, &bucket.BucketOption)
		if err != nil {
			return err
		}
		dp = ndp
	}

	totalSize := 0

	stripeCount := 0
	sendCount := 0
	rawLen := 0
	opID := bucket.NextOpID
	dp.dv.Reset()

	buf := make([]byte, dp.stripeSize)
	rdata := make([]byte, dp.stripeSize)
	curStripe := bucket.Length / uint64(dp.stripeSize)

	tr := etag.NewTree()

	h := md5.New()
	/*
		if opts.UserDefined != nil {
			val, ok := opts.UserDefined["etag"]
			if ok && val == "cid" {
				h = sha256.New()
			}
		}
	*/

	breakFlag := false
	for !breakFlag {
		logger.Debug("upload stripe: ", curStripe, stripeCount)
		// clear itself
		buf = buf[:0]

		// 读数据，限定一次处理一个stripe
		n, err := io.ReadAtLeast(r, rdata[:dp.stripeSize], dp.stripeSize)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			breakFlag = true
		} else if err != nil {
			return err
		} else if n != dp.stripeSize {
			logger.Debug("fail to get enough data")
			return xerrors.New("upload fails due to read io")
		}

		buf = append(buf, rdata[:n]...)
		bufLen := len(buf)
		if bufLen == 0 {
			break
		}

		// hash of raw data
		h.Write(buf)
		if opts.UserDefined != nil {
			val, ok := opts.UserDefined["etag"]
			if ok && val == "cid" {
				for start := 0; start < bufLen; {
					stepLen := build.DefaultSegSize
					if start+stepLen > bufLen {
						stepLen = bufLen - start
					}
					cid := etag.NewCidFromData(buf[start : start+stepLen])

					tr.AddCid(cid, uint64(stepLen))

					start += stepLen
				}
			}
		}

		rawLen += len(buf)
		totalSize += len(buf)

		// encrypt
		if object.Encryption == "aes" {
			if len(buf)%aes.BlockSize != 0 {
				buf = aes.PKCS5Padding(buf)
			}

			aesEnc, err := aes.ContructAesEnc(dp.aesKey[:], object.ObjectID, curStripe+uint64(stripeCount))
			if err != nil {
				return err
			}

			crypted := make([]byte, len(buf))
			aesEnc.CryptBlocks(crypted, buf)
			copy(buf, crypted)
		}

		dp.segID.SetStripeID(curStripe + uint64(stripeCount))

		encodedData, err := dp.coder.Encode(dp.segID, buf)
		if err != nil {
			logger.Debug("encode data error:", dp.segID, err)
			return err
		}

		for i := 0; i < (dp.dataCount + dp.parityCount); i++ {
			dp.segID.SetChunkID(uint32(i))

			seg := segment.NewBaseSegment(encodedData[i], dp.segID)

			segData, _ := seg.Content()
			segTag, _ := seg.Tags()

			err := dp.dv.Add(dp.segID.Bytes(), segData, segTag[0])
			if err != nil {
				logger.Debug("Process data error:", dp.segID, err)
			}

			err = l.OrderMgr.PutSegmentToLocal(ctx, seg)
			if err != nil {
				logger.Debug("Process data error:", dp.segID, err)
				return err
			}
		}
		stripeCount++
		sendCount++
		// send some to order; 64MB
		if sendCount*int(bucket.DataCount+bucket.ParityCount) >= 256 || stripeCount*int(bucket.DataCount+bucket.ParityCount) >= 4096 || breakFlag {
			ok, err := dp.dv.Result()
			if !ok || err != nil {
				return xerrors.New("encode data is wrong")
			}

			// put to local first
			sjl := &types.SegJob{
				JobID:    opID,
				BucketID: object.BucketID,
				Start:    curStripe,
				Length:   uint64(stripeCount),
				ChunkID:  bucket.DataCount + bucket.ParityCount,
			}

			data, err := sjl.Serialize()
			if err != nil {
				return err
			}

			key := store.NewKey(pb.MetaType_LFS_OpJobsKey, l.userID, object.BucketID, opID)
			l.ds.Put(key, data)

			// send out to order manager
			sj := &types.SegJob{
				BucketID: object.BucketID,
				JobID:    opID,
				Start:    curStripe + uint64(stripeCount-sendCount),
				Length:   uint64(sendCount),
				ChunkID:  bucket.DataCount + bucket.ParityCount,
			}

			logger.Debug("send job to order manager: ", sj.BucketID, opID, sj.Start, sj.Length, sj.ChunkID)
			l.OrderMgr.AddSegJob(sj)

			sendCount = 0

			// update
			dp.dv.Reset()
		}

		// more, change opID; 1GB
		if stripeCount*int(bucket.DataCount+bucket.ParityCount) >= 4096 || breakFlag {
			etagb := h.Sum(nil)
			if opts.UserDefined != nil {
				val, ok := opts.UserDefined["etag"]
				if ok && val == "cid" {
					if breakFlag {
						cidEtag := tr.Root()
						etagb = cidEtag.Bytes()
					} else {
						cidEtag := tr.TmpRoot()
						etagb = cidEtag.Bytes()
					}
				}
			}

			usedBytes := uint64(dp.stripeSize * (dp.dataCount + dp.parityCount) * stripeCount / dp.dataCount)
			opi := &pb.ObjectPartInfo{
				ObjectID:    object.GetObjectID(),
				Time:        time.Now().Unix(),
				Offset:      uint64(dp.stripeSize) * curStripe,
				Length:      uint64(rawLen), // file size
				StoredBytes: usedBytes,      // used bytes
				ETag:        etagb,
			}

			payload, err := proto.Marshal(opi)
			if err != nil {
				return err
			}
			op := &pb.OpRecord{
				Type:    pb.OpRecord_AddData,
				Payload: payload,
			}

			// todo: add used bytes
			bucket.Length += uint64(dp.stripeSize * stripeCount)
			bucket.UsedBytes += usedBytes

			err = bucket.addOpRecord(l.userID, op, l.ds)
			if err != nil {
				return err
			}

			object.ops = append(object.ops, op.OpID)
			object.dirty = true
			object.addPartInfo(opi)
			err = object.Save(l.userID, l.ds)
			if err != nil {
				return err
			}

			opID++
			curStripe += uint64(stripeCount)

			// not reset to get hash
			//h.Reset()
			rawLen = 0
			stripeCount = 0
		}
	}

	logger.Debug("upload end at: ", time.Now())
	logger.Debug("upload: ", totalSize, ", cost: ", time.Since(nt))

	return nil
}

func (l *LfsService) download(ctx context.Context, dp *dataProcess, bucket *bucket, object *object, start, length int, w io.Writer) error {
	logger.Debug("download object: ", object.BucketID, object.ObjectID, start, length)

	sizeReceived := 0

	// todo: parallel download stripe
	breakFlag := false
	for !breakFlag {
		select {
		case <-ctx.Done():
			return xerrors.Errorf("context is cancle or done")
		default:
			stripeID := start / dp.stripeSize

			logger.Debug("download object stripe: ", object.BucketID, object.ObjectID, stripeID)

			// add parallel chunks download
			// release when get chunk fails or get datacount chunk succcess
			sm := semaphore.NewWeighted(int64(dp.dataCount))
			var wg sync.WaitGroup
			sucCnt := int32(0)
			failCnt := int32(0)

			stripe := make([][]byte, dp.dataCount+dp.parityCount)
			for i := 0; i < dp.dataCount+dp.parityCount; i++ {
				//logger.Debug("download segment: ", bucket.BucketID, uint64(stripeID), uint32(i))
				err := sm.Acquire(ctx, 1)
				if err != nil {
					return err
				}

				// fails too many, no need to download
				if atomic.LoadInt32(&failCnt) > int32(dp.parityCount) {
					logger.Warn("download chunk failed too much")
					break
				}

				// enough, no need to download
				if atomic.LoadInt32(&sucCnt) >= int32(dp.dataCount) {
					break
				}

				wg.Add(1)
				go func(chunkID int) {
					defer wg.Done()

					segID, err := segment.NewSegmentID(l.fsID, bucket.BucketID, uint64(stripeID), uint32(chunkID))
					if err != nil {
						atomic.AddInt32(&failCnt, 1)
						sm.Release(1)
						return
					}

					seg, err := l.OrderMgr.GetSegment(ctx, segID)
					if err != nil {
						atomic.AddInt32(&failCnt, 1)
						sm.Release(1)
						logger.Debug("download chunk fail: ", segID, err)
						return
					}

					// verify seg
					ok, err := dp.coder.VerifyChunk(segID, seg.Data())
					if err != nil || !ok {
						atomic.AddInt32(&failCnt, 1)
						sm.Release(1)
						logger.Debug("download chunk is wrong: ", chunkID, segID, err)
						return
					}

					logger.Debug("download success: ", chunkID, segID)
					stripe[chunkID] = seg.Data()
					atomic.AddInt32(&sucCnt, 1)

					// download dataCount; release resource to break
					if atomic.LoadInt32(&sucCnt) >= int32(dp.dataCount) {
						sm.Release(1)
					}
				}(i)
			}
			wg.Wait()

			if sucCnt < int32(dp.dataCount) {
				// retry?
				failCnt = int32(0)
				sm := semaphore.NewWeighted(int64(dp.dataCount))
				for i := 0; i < dp.dataCount+dp.parityCount; i++ {
					err := sm.Acquire(ctx, 1)
					if err != nil {
						return err
					}

					// has data
					if len(stripe[i]) > 0 {
						continue
					}

					// fails too many, no need to download
					if atomic.LoadInt32(&failCnt) > int32(dp.parityCount) {
						logger.Warn("download chunk failed too much")
						break
					}

					// enough, no need to download
					if atomic.LoadInt32(&sucCnt) >= int32(dp.dataCount) {
						break
					}

					wg.Add(1)
					go func(chunkID int) {
						defer wg.Done()

						segID, err := segment.NewSegmentID(l.fsID, bucket.BucketID, uint64(stripeID), uint32(chunkID))
						if err != nil {
							atomic.AddInt32(&failCnt, 1)
							sm.Release(1)
							return
						}

						seg, err := l.OrderMgr.GetSegment(ctx, segID)
						if err != nil {
							atomic.AddInt32(&failCnt, 1)
							sm.Release(1)
							logger.Debug("download chunk fail: ", segID, err)
							return
						}

						// verify seg
						ok, err := dp.coder.VerifyChunk(segID, seg.Data())
						if err != nil || !ok {
							atomic.AddInt32(&failCnt, 1)
							sm.Release(1)
							logger.Debug("download chunk is wrong: ", chunkID, segID, err)
							return
						}

						logger.Debug("download success: ", chunkID, segID)
						stripe[chunkID] = seg.Data()
						atomic.AddInt32(&sucCnt, 1)

						// download dataCount; release resource to break
						if atomic.LoadInt32(&sucCnt) >= int32(dp.dataCount) {
							sm.Release(1)
						}
					}(i)
				}
				wg.Wait()

				// check again
				if sucCnt < int32(dp.dataCount) {
					return xerrors.Errorf("download not get enough chunks, expected %d, got %d", dp.dataCount, sucCnt)
				}
			}

			res, err := dp.coder.Decode(nil, stripe)
			if err != nil {
				logger.Debug("download decode object error: ", object.BucketID, object.ObjectID, stripeID, err)
				return err
			}

			if object.Encryption == "aes" {
				aesDec, err := aes.ContructAesDec(dp.aesKey[:], object.ObjectID, uint64(stripeID))
				if err != nil {
					return err
				}
				decrypted := make([]byte, len(res))
				aesDec.CryptBlocks(decrypted, res)
				res = decrypted
			}

			stripeOffset := start % dp.stripeSize

			rLen := dp.stripeSize - stripeOffset

			// read to end
			if rLen > length-sizeReceived {
				rLen = length - sizeReceived
			}

			wl, err := w.Write(res[stripeOffset : stripeOffset+rLen])
			if err != nil {
				return err
			}

			if wl != rLen {
				logger.Warn("download: write length is not equal")
			}

			start += rLen
			sizeReceived += rLen
			if sizeReceived >= length {
				breakFlag = true
			}
		}
	}

	return nil
}
