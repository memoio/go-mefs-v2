package lfs

import (
	"context"
	"crypto/cipher"
	"crypto/md5"
	"encoding/binary"
	"io"
	"log"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/zeebo/blake3"

	"github.com/memoio/go-mefs-v2/lib/code"
	"github.com/memoio/go-mefs-v2/lib/crypto/aes"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
)

type dataProcess struct {
	sync.Mutex

	bucketID    uint64
	dataCount   int
	parityCount int
	stripeSize  int // dataCount*segSize

	aesKey [32]byte // aes encrypt
	coder  code.Codec
	segID  segment.SegmentID // for encode

	keyset pdpcommon.KeySet // only verify? not needed
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
		bucketID:   bucketID,
		aesKey:     hres,
		coder:      coder,
		stripeSize: stripeSize,
		segID:      segID,
	}

	l.Lock()
	l.dps[bucketID] = dp
	l.Unlock()
	return dp, nil
}

func (l *LfsService) upload(ctx context.Context, bucket *bucket, object *object, r io.Reader) error {
	dp, ok := l.dps[bucket.BucketID]
	if !ok {
		ndp, err := l.newDataProcess(bucket.BucketID, &bucket.BucketOption)
		if err != nil {
			return err
		}
		dp = ndp
	}

	stripeCount := 0
	rawLen := 0

	buf := make([]byte, dp.stripeSize)
	rdata := make([]byte, dp.stripeSize)
	curStripe := 1 + (bucket.Length-1)/uint64(dp.stripeSize)

	var aesEnc cipher.BlockMode
	if object.Encryption == "aes" {
		tmpkey := make([]byte, 40)
		copy(tmpkey, dp.aesKey[:])
		binary.BigEndian.PutUint64(tmpkey[32:], object.ObjectID)
		hres := blake3.Sum256(tmpkey)

		tmpEnc, err := aes.ContructAesEnc(hres[:])
		if err != nil {
			return err
		}
		aesEnc = tmpEnc
	}

	h := md5.New()

	breakFlag := false
	for !breakFlag {
		select {
		case <-ctx.Done():
			log.Println("upload cancel")
			return nil
		default:
			// clear itself
			buf = buf[:0]

			// 读数据，限定一次处理一个stripe
			n, err := io.ReadAtLeast(r, rdata[:dp.stripeSize], dp.stripeSize)
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				breakFlag = true
			} else if err != nil {
				return err
			} else if n != dp.stripeSize {
				return ErrUpload
			}

			buf = append(buf, rdata[:n]...)
			if len(buf) == 0 {
				break
			}

			// md5 of raw data
			h.Write(buf)
			rawLen += len(buf)

			// encrypt
			if aesEnc != nil {
				if len(buf)%aes.BlockSize != 0 {
					buf = aes.PKCS5Padding(buf)
				}
				crypted := make([]byte, len(buf))
				aesEnc.CryptBlocks(crypted, buf)
				copy(buf, crypted)
			}

			dp.segID.SetStripeID(curStripe + uint64(stripeCount))

			encodedData, err := dp.coder.Encode(dp.segID, buf)
			if err != nil {
				return err
			}

			for i := 0; i < (dp.dataCount + dp.parityCount); i++ {
				dp.segID.SetChunkID(uint32(i))

				seg := segment.NewBaseSegment(encodedData[i], dp.segID)

				segData, _ := seg.SegData()
				segTag, _ := seg.Tag()

				ok, err := dp.keyset.PublicKey().VerifyTag(dp.segID.Bytes(), segData, segTag, 31)
				if !ok || err != nil {
					log.Println("Process data error:", dp.segID.String(), err)
				}
				err = l.segStore.Put(seg)
				if err != nil {
					return err
				}
			}
			stripeCount++

			if stripeCount > 10 || breakFlag {
				opi := &pb.ObjectPartInfo{
					ObjectID:  object.GetObjectID(),
					Time:      time.Now().Unix(),
					Offset:    curStripe,
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
					OpID:    bucket.NextOpID,
					Payload: payload,
				}

				err = saveOpRecord(l.userID, bucket.BucketID, op, l.ds)
				if err != nil {
					return err
				}

				bucket.NextOpID++
				bucket.Length += uint64(dp.stripeSize * stripeCount)
				bucket.dirty = true
				err = bucket.Save(l.userID, l.ds)
				if err != nil {
					return err
				}

				object.ops = append(object.ops, op.OpID)
				object.addPartInfo(opi)
				err = object.Save(l.userID, l.ds)
				if err != nil {
					return err
				}

				// send out

				// update data
				h.Reset()
				rawLen = 0
				curStripe += uint64(stripeCount)
				stripeCount = 0
			}
		}
	}

	return nil
}

func (l *LfsService) download(ctx context.Context, dp *dataProcess, aesDec cipher.BlockMode, bucket *bucket, object *object, start, length int, w io.Writer) error {
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
			start += sizeReceived
			stripeID := start / dp.stripeSize

			segID.SetStripeID(uint64(stripeID))
			stripe := make([][]byte, dp.dataCount+dp.parityCount)
			for i := 0; i < dp.dataCount+dp.parityCount; i++ {
				if sucCount >= dp.dataCount {
					break
				}

				segID.SetChunkID(uint32(i))
				// get from remote
				var chunk []byte

				log.Println("receive chunk len:", len(chunk))
				stripe[i] = chunk
				sucCount++
			}

			res, err := dp.coder.Decode(nil, stripe)
			if err != nil {
				continue
			}

			if aesDec != nil {
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

			sizeReceived += rLen
			if sizeReceived >= length {
				breakFlag = true
			}
		}
	}

	return nil
}
