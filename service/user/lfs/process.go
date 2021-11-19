package lfs

import (
	"context"
	"crypto/md5"
	"encoding/binary"
	"io"
	"sync"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/gogo/protobuf/proto"
	"github.com/zeebo/blake3"

	"github.com/memoio/go-mefs-v2/lib/code"
	"github.com/memoio/go-mefs-v2/lib/crypto/aes"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	pdpv2 "github.com/memoio/go-mefs-v2/lib/crypto/pdp/version2"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
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

	dp := &dataProcess{
		bucketID:    bucketID,
		dataCount:   int(bopt.DataCount),
		parityCount: int(bopt.ParityCount),
		stripeSize:  stripeSize,

		aesKey: hres,
		coder:  coder,
		segID:  segID,

		dv: pdpv2.NewDataVerifier(l.keyset.PublicKey(), l.keyset.SecreteKey()),
	}

	l.Lock()
	l.dps[bucketID] = dp
	l.Unlock()
	return dp, nil
}

func (l *LfsService) upload(ctx context.Context, bucket *bucket, object *object, r io.Reader) error {
	nt := time.Now()
	logger.Debug("upload begin at: ", nt)
	dp, ok := l.dps[bucket.BucketID]
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

	h := md5.New()

	breakFlag := false
	for !breakFlag {
		select {
		case <-ctx.Done():
			logger.Warn("upload cancel")
			return nil
		default:
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
				logger.Warn("fail to get enough data")
				return ErrUpload
			}

			buf = append(buf, rdata[:n]...)
			if len(buf) == 0 {
				break
			}

			// md5 of raw data
			h.Write(buf)
			rawLen += len(buf)
			totalSize += len(buf)

			// encrypt
			if object.Encryption == "aes" {
				if len(buf)%aes.BlockSize != 0 {
					buf = aes.PKCS5Padding(buf)
				}
				crypted := make([]byte, len(buf))

				tmpkey := make([]byte, 48)
				copy(tmpkey, dp.aesKey[:])
				binary.BigEndian.PutUint64(tmpkey[32:], object.ObjectID)
				binary.BigEndian.PutUint64(tmpkey[40:], curStripe+uint64(stripeCount))
				hres := blake3.Sum256(tmpkey)

				aesEnc, err := aes.ContructAesEnc(hres[:])
				if err != nil {
					return err
				}

				aesEnc.CryptBlocks(crypted, buf)
				copy(buf, crypted)
			}

			dp.segID.SetStripeID(curStripe + uint64(stripeCount))

			encodedData, err := dp.coder.Encode(dp.segID, buf)
			if err != nil {
				logger.Warn("encode data error:", dp.segID.String(), err)
				return err
			}

			for i := 0; i < (dp.dataCount + dp.parityCount); i++ {
				dp.segID.SetChunkID(uint32(i))

				seg := segment.NewBaseSegment(encodedData[i], dp.segID)

				segData, _ := seg.Content()
				segTag, _ := seg.Tags()

				err := dp.dv.Input(dp.segID.Bytes(), segData, segTag[0])
				if err != nil {
					logger.Warn("Process data error:", dp.segID.String(), err)
				}

				err = l.om.PutSegmentToLocal(ctx, seg)
				if err != nil {
					logger.Warn("Process data error:", dp.segID.String(), err)
					return err
				}
			}
			stripeCount++
			sendCount++
			// send some to order
			if sendCount >= 16 || breakFlag {
				ok := dp.dv.Result()
				if !ok {
					return ErrEncode
				}

				// put to local first
				sjl := &types.SegJob{
					JobID:    opID,
					BucketID: object.BucketID,
					Start:    curStripe,
					Length:   uint64(stripeCount),
					ChunkID:  bucket.DataCount + bucket.ParityCount,
				}

				data, _ := cbor.Marshal(sjl)
				key := store.NewKey(pb.MetaType_LFS_OpJobsKey, l.userID, object.BucketID, opID)
				err := l.ds.Put(key, data)
				if err != nil {
					continue
				}

				// send out
				sj := &types.SegJob{
					JobID:    opID,
					BucketID: object.BucketID,
					Start:    curStripe + uint64(stripeCount-sendCount),
					Length:   uint64(sendCount),
					ChunkID:  bucket.DataCount + bucket.ParityCount,
				}

				logger.Debug("send job to order: ", opID, sj.Start, sj.Length)
				l.om.AddSegJob(sj)
				logger.Debug("send job to order finish: ", opID, sj.Start, sj.Length)

				sendCount = 0

				// update
				dp.dv.Reset()
			}

			// more, change opID
			if stripeCount >= 64 || breakFlag {
				opi := &pb.ObjectPartInfo{
					ObjectID:  object.GetObjectID(),
					Time:      time.Now().Unix(),
					Offset:    uint64(dp.stripeSize) * curStripe,
					Length:    uint64(dp.stripeSize * stripeCount),
					RawLength: uint64(rawLen),
					ETag:      h.Sum(nil),
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

				h.Reset()
				rawLen = 0
				stripeCount = 0
			}
		}
	}

	logger.Debug("upload end at: ", time.Now())
	logger.Debug("upload: ", totalSize, ", cost: ", time.Since(nt))

	return nil
}

func (l *LfsService) download(ctx context.Context, dp *dataProcess, bucket *bucket, object *object, start, length int, w io.Writer) error {

	sizeReceived := 0
	sucCount := 0

	segID, err := segment.NewSegmentID(l.fsID, bucket.BucketID, 0, 0)
	if err != nil {
		return err
	}

	breakFlag := false
	for !breakFlag {
		select {
		case <-ctx.Done():
			return nil
		default:
			sucCount = 0
			stripeID := start / dp.stripeSize

			logger.Debug("download object: ", object.BucketID, object.ObjectID, stripeID)

			segID.SetStripeID(uint64(stripeID))
			stripe := make([][]byte, dp.dataCount+dp.parityCount)
			for i := 0; i < dp.dataCount+dp.parityCount; i++ {
				if sucCount >= dp.dataCount {
					break
				}

				segID.SetChunkID(uint32(i))
				seg, err := l.om.GetSegment(ctx, segID)
				if err != nil {
					logger.Debug("get seg error:", err)
					continue
				}

				stripe[i] = seg.Data()
				sucCount++
			}

			res, err := dp.coder.Decode(nil, stripe)
			if err != nil {
				logger.Debug("decode object error: ", object.BucketID, object.ObjectID, stripeID, err)
				return err
			}

			if object.Encryption == "aes" {
				tmpkey := make([]byte, 48)
				copy(tmpkey, dp.aesKey[:])
				binary.BigEndian.PutUint64(tmpkey[32:], object.ObjectID)
				binary.BigEndian.PutUint64(tmpkey[40:], uint64(stripeID))
				hres := blake3.Sum256(tmpkey)

				aesDec, err := aes.ContructAesDec(hres[:])
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
