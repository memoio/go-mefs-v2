package lfs

import (
	"context"
	"crypto/md5"
	"io"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gogo/protobuf/proto"
	"golang.org/x/sync/semaphore"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/code"
	"github.com/memoio/go-mefs-v2/lib/crypto/aes"
	"github.com/memoio/go-mefs-v2/lib/crypto/pdp"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	"github.com/memoio/go-mefs-v2/lib/etag"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/lib/utils"
)

type dataProcess struct {
	sync.Mutex

	userID      uint64
	fsID        []byte
	bucketID    uint64
	dataCount   int
	parityCount int
	stripeSize  int // dataCount*segSize

	aesKey [32]byte // aes decrypt key
	coder  code.Codec

	keyset pdpcommon.KeySet
}

func (l *LfsService) getDataProcess(ctx context.Context, userID, bucketID uint64, bopt *pb.BucketOption) (*dataProcess, error) {
	if userID == l.userID {
		l.RLock()
		dp, ok := l.dps[bucketID]
		l.RUnlock()
		if ok {
			return dp, nil
		}
	}

	dp := &dataProcess{
		userID:      userID,
		bucketID:    bucketID,
		dataCount:   int(bopt.DataCount),
		parityCount: int(bopt.ParityCount),
		stripeSize:  int(bopt.SegSize) * int(bopt.DataCount),

		fsID:   l.fsID,
		keyset: l.keyset,
	}

	if userID != l.userID {
		g, err := l.getGhost(ctx, userID)
		if err != nil {
			return nil, err
		}
		dp.keyset = g.keyset
		dp.fsID = g.fsID
	}

	coder, err := code.NewDataCoderWithBopts(dp.keyset, bopt)
	if err != nil {
		return nil, err
	}
	dp.coder = coder

	if userID == l.userID {
		l.Lock()
		l.dps[bucketID] = dp
		l.Unlock()
	}

	return dp, nil
}

func (l *LfsService) upload(ctx context.Context, bucket *bucket, object *object, r io.Reader, opts types.PutObjectOptions) error {
	nt := time.Now()
	logger.Debug("upload begin at: ", nt)

	dp, err := l.getDataProcess(ctx, l.userID, bucket.BucketID, &bucket.BucketOption)
	if err != nil {
		return err
	}

	dv, err := pdp.NewDataVerifier(dp.keyset.PublicKey(), dp.keyset.SecreteKey())
	if err != nil {
		return err
	}

	totalSize := 0

	stripeCount := 0
	sendCount := 0
	rawLen := 0
	opID := bucket.NextOpID

	buf := make([]byte, dp.stripeSize)
	rdata := make([]byte, dp.stripeSize)
	curStripe := bucket.Length / uint64(dp.stripeSize) // length is aligned
	segID, err := segment.NewSegmentID(dp.fsID, dp.bucketID, 0, 0)
	if err != nil {
		return err
	}

	h := md5.New()
	if opts.UserDefined != nil {
		etags := opts.UserDefined["etag"]
		if strings.HasPrefix(etags, "cid") {
			c := &etag.Config{
				BlockSize: build.DefaultSegSize,
			}
			etagss := strings.Split(etags, "-")
			if len(etagss) > 1 {
				c.BlockSize = int(utils.HumanStringLoaded(etagss[1]))
				if c.BlockSize < 1024 {
					c.BlockSize = build.DefaultSegSize
				}
			}
			h = etag.NewTreeWithConfig(c)
		}
	}

	breakFlag := false
	for !breakFlag {
		logger.Debug("upload stripe: ", curStripe, stripeCount)
		// clear itself
		buf = buf[:0]

		// process one stripe
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

		h.Write(buf)

		rawLen += len(buf)
		totalSize += len(buf)

		// encrypt
		switch object.Encryption {
		case "aes": // each stripe has one encrypt code, wrong code here for compatible
			if len(buf)%aes.BlockSize != 0 {
				buf = aes.PKCS5Padding(buf)
			}

			aesKey := aes.ContructAesKey(nil, dp.bucketID, object.ObjectID, curStripe+uint64(stripeCount))

			aesEnc, err := aes.ContructAesEnc(aesKey)
			if err != nil {
				return err
			}

			crypted := make([]byte, len(buf))
			aesEnc.CryptBlocks(crypted, buf)
			copy(buf, crypted)
		case "aes1": // each bucket has one encrypt code
			if len(buf)%aes.BlockSize != 0 {
				buf = aes.PKCS5Padding(buf)
			}

			aesKey := aes.ContructAesKey(l.encryptKey, dp.bucketID, math.MaxUint64, math.MaxUint64)
			aesEnc, err := aes.ContructAesEnc(aesKey)
			if err != nil {
				return err
			}

			crypted := make([]byte, len(buf))
			aesEnc.CryptBlocks(crypted, buf)
			copy(buf, crypted)
		case "aes2": // each obejct has one encrypt code
			if len(buf)%aes.BlockSize != 0 {
				buf = aes.PKCS5Padding(buf)
			}

			aesKey := aes.ContructAesKey(l.encryptKey, dp.bucketID, object.ObjectID, math.MaxUint64)
			aesEnc, err := aes.ContructAesEnc(aesKey)
			if err != nil {
				return err
			}

			crypted := make([]byte, len(buf))
			aesEnc.CryptBlocks(crypted, buf)
			copy(buf, crypted)
		case "aes3": // each stripe has one encryt code
			if len(buf)%aes.BlockSize != 0 {
				buf = aes.PKCS5Padding(buf)
			}

			aesKey := aes.ContructAesKey(l.encryptKey, dp.bucketID, object.ObjectID, curStripe+uint64(stripeCount))
			aesEnc, err := aes.ContructAesEnc(aesKey)
			if err != nil {
				return err
			}

			crypted := make([]byte, len(buf))
			aesEnc.CryptBlocks(crypted, buf)
			copy(buf, crypted)
		default:
		}

		segID.SetStripeID(curStripe + uint64(stripeCount))

		encodedData, err := dp.coder.Encode(segID, buf)
		if err != nil {
			logger.Debug("encode data error: ", segID, err)
			return err
		}

		for i := 0; i < (dp.dataCount + dp.parityCount); i++ {
			segID.SetChunkID(uint32(i))

			seg := segment.NewBaseSegment(encodedData[i], segID)

			segData, err := seg.Content()
			if err != nil {
				return err
			}
			segTag, err := seg.Tags()
			if err != nil {
				return err
			}
			err = dv.Add(segID.Bytes(), segData, segTag[0])
			if err != nil {
				return err
			}

			err = l.OrderMgr.PutSegmentToLocal(ctx, seg)
			if err != nil {
				return err
			}
		}
		stripeCount++
		sendCount++
		// send some to order; 4MB*(bucket.DataCount + bucket.ParityCount)
		if sendCount >= 16 || breakFlag {
			ok, err := dv.Result()
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
			dv.Reset()

			etagb := h.Sum(nil)

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

			bucket.Length += uint64(dp.stripeSize * stripeCount)
			bucket.UsedBytes += usedBytes

			err = l.addOpRecord(bucket, op)
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

func (l *LfsService) download(ctx context.Context, dp *dataProcess, dv pdpcommon.DataVerifier, bi types.BucketInfo, object *object, start, length int, w io.Writer) error {
	logger.Debug("download object: ", object.BucketID, object.ObjectID, start, length)

	sizeReceived := 0

	// TODO: parallel download stripe
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
			var wg sync.WaitGroup
			sucCnt := int32(0)
			failCnt := int32(0)

			// accumulate verify
			var lk sync.Mutex
			dv.Reset()

			stripe := make([][]byte, dp.dataCount+dp.parityCount)

			sm := semaphore.NewWeighted(int64(dp.dataCount))
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

					segID, err := segment.NewSegmentID(dp.fsID, bi.BucketID, uint64(stripeID), uint32(chunkID))
					if err != nil {
						atomic.AddInt32(&failCnt, 1)
						sm.Release(1)
						return
					}

					seg, err := l.getSegment(ctx, dp.userID, segID)
					if err != nil {
						atomic.AddInt32(&failCnt, 1)
						sm.Release(1)
						logger.Debug("download chunk fail: ", segID, err)
						return
					}

					lk.Lock()
					defer lk.Unlock()
					da, err := seg.Content()
					if err != nil {
						atomic.AddInt32(&failCnt, 1)
						sm.Release(1)
						logger.Debug("download chunk fail: ", segID, err)
						return
					}

					tags, err := seg.Tags()
					if err != nil {
						atomic.AddInt32(&failCnt, 1)
						sm.Release(1)
						logger.Debug("download chunk fail: ", segID, err)
						return
					}

					err = dv.Add(segID.Bytes(), da, tags[0])
					if err != nil {
						atomic.AddInt32(&failCnt, 1)
						sm.Release(1)
						logger.Debug("download chunk fail: ", segID, err)
						return
					}

					logger.Debug("download chunk success: ", segID)
					stripe[chunkID] = seg.Data()

					atomic.AddInt32(&sucCnt, 1)

					// download dataCount; release resource to break
					if atomic.LoadInt32(&sucCnt) >= int32(dp.dataCount) {
						sm.Release(1)
					}
				}(i)
			}
			wg.Wait()

			var aerr error
			if sucCnt >= int32(dp.dataCount) {
				ok, err := dv.Result()
				if !ok {
					aerr = xerrors.Errorf("download contains wrong chunk")
				}
				if err != nil {
					aerr = err
				}
			}

			if sucCnt < int32(dp.dataCount) || aerr != nil {
				logger.Debug("retry download stripe: ", object.BucketID, object.ObjectID, stripeID, aerr)
				l.addSegLoc(ctx, dp.userID)

				// retry
				failCnt = 0
				sucCnt = 0
				sm := semaphore.NewWeighted(int64(dp.dataCount))
				for i := 0; i < dp.dataCount+dp.parityCount; i++ {
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

						segID, err := segment.NewSegmentID(dp.fsID, bi.BucketID, uint64(stripeID), uint32(chunkID))
						if err != nil {
							atomic.AddInt32(&failCnt, 1)
							sm.Release(1)
							return
						}

						// has data, parallel verify
						if len(stripe[chunkID]) > 0 {
							ok, err := dp.coder.VerifyChunk(segID, stripe[chunkID])
							if err == nil && ok {
								logger.Debug("download good stripe chunk: ", stripeID, chunkID)
								atomic.AddInt32(&sucCnt, 1)

								if atomic.LoadInt32(&sucCnt) >= int32(dp.dataCount) {
									sm.Release(1)
								}

								return
							} else {
								logger.Debug("download bad stripe chunk: ", stripeID, chunkID)
								stripe[chunkID] = nil
								l.OrderMgr.DeleteSegment(ctx, segID)
								// handle provider, sub credit
							}
						}

						seg, err := l.getSegment(ctx, dp.userID, segID)
						if err != nil {
							atomic.AddInt32(&failCnt, 1)
							sm.Release(1)
							logger.Debug("download chunk fail: ", segID, err)
							return
						}

						// verify each seg
						ok, err := dp.coder.VerifyChunk(segID, seg.Data())
						if err != nil || !ok {
							atomic.AddInt32(&failCnt, 1)
							sm.Release(1)
							l.OrderMgr.DeleteSegment(ctx, segID)
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

			switch object.Encryption {
			case "aes":
				aesKey := aes.ContructAesKey(nil, dp.bucketID, object.ObjectID, uint64(stripeID))
				aesDec, err := aes.ContructAesDec(aesKey)
				if err != nil {
					return err
				}
				decrypted := make([]byte, len(res))
				aesDec.CryptBlocks(decrypted, res)
				res = decrypted
			case "aes1":
				aesKey := dp.aesKey[:]
				if dp.userID == l.userID {
					aesKey = aes.ContructAesKey(l.encryptKey, dp.bucketID, math.MaxUint64, math.MaxUint64)
				}
				aesDec, err := aes.ContructAesDec(aesKey[:])
				if err != nil {
					return err
				}
				decrypted := make([]byte, len(res))
				aesDec.CryptBlocks(decrypted, res)
				res = decrypted
			case "aes2":
				aesKey := dp.aesKey[:]
				if dp.userID == l.userID {
					aesKey = aes.ContructAesKey(l.encryptKey, dp.bucketID, object.ObjectID, math.MaxUint64)
				}
				aesDec, err := aes.ContructAesDec(aesKey[:])
				if err != nil {
					return err
				}
				decrypted := make([]byte, len(res))
				aesDec.CryptBlocks(decrypted, res)
				res = decrypted
			case "aes3":
				aesKey := aes.ContructAesKey(l.encryptKey, dp.bucketID, object.ObjectID, uint64(stripeID))
				aesDec, err := aes.ContructAesDec(aesKey[:])
				if err != nil {
					return err
				}
				decrypted := make([]byte, len(res))
				aesDec.CryptBlocks(decrypted, res)
				res = decrypted
			default:
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
