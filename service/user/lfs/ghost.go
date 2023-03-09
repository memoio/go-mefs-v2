package lfs

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/memoio/go-mefs-v2/lib/crypto/pdp"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	"github.com/memoio/go-mefs-v2/lib/etag"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"golang.org/x/xerrors"
)

type ghost struct {
	fsID   []byte
	keyset pdpcommon.KeySet

	segloc bool
}

func (l *LfsService) getGhost(ctx context.Context, userID uint64) (*ghost, error) {
	if userID == l.userID {
		return &ghost{
			keyset: l.keyset,
			fsID:   l.keyset.VerifyKey().Hash(),
		}, nil
	}

	g, ok := l.users[userID]
	if ok {
		return g, nil
	}

	pbyte, err := l.StateGetPDPPublicKey(ctx, userID)
	if err != nil {
		return nil, err
	}

	keyset, err := pdp.ReKeyWithPublicKey(pdpcommon.PDPV2, pbyte)
	if err != nil {
		return nil, err
	}

	g = &ghost{
		keyset: keyset,
		fsID:   keyset.VerifyKey().Hash(),
	}

	l.Lock()
	l.users[userID] = g
	l.Unlock()

	return g, nil
}

func (l *LfsService) getOther(ctx context.Context, cidName string, opts types.DownloadObjectOptions) (*tx.ObjMetaKey, error) {
	tag, err := etag.ToByte(cidName)
	if err != nil {
		return nil, err
	}

	omv := &tx.ObjMetaKey{
		UserID:   math.MaxUint64,
		BucketID: math.MaxUint64,
		ObjectID: math.MaxUint64,
	}

	if opts.UserDefined != nil {
		if opts.UserDefined["userID"] != "" {
			omv.UserID, _ = strconv.ParseUint(opts.UserDefined["userID"], 10, 0)
		}
		if opts.UserDefined["bucketID"] != "" {
			omv.BucketID, _ = strconv.ParseUint(opts.UserDefined["bucketID"], 10, 0)
		}
		if opts.UserDefined["objectID"] != "" {
			omv.ObjectID, _ = strconv.ParseUint(opts.UserDefined["objectID"], 10, 0)
		}
	}

	if omv.UserID != math.MaxUint64 && omv.BucketID != math.MaxUint64 && omv.ObjectID != math.MaxUint64 {
		return omv, nil
	}

	omki, ok := l.tagCache.Get(cidName)
	if ok {
		omk, ok := omki.(*tx.ObjMetaKey)
		if ok {
			if omv.UserID != math.MaxUint64 {
				if omv.UserID == omk.UserID {
					if omv.BucketID != math.MaxUint64 {
						if omv.BucketID == omk.BucketID {
							return omk, nil
						}
					} else {
						return omk, nil
					}
				}
			} else {

				return omk, nil
			}
		}
	}

	i := uint64(0)
	for {
		omk, err := l.StateGetObjMetaKey(ctx, tag, i)
		if err != nil {
			return nil, err
		}

		l.tagCache.Add(cidName, omk)

		if omv.UserID != math.MaxUint64 {
			if omv.UserID == omk.UserID {
				if omv.BucketID != math.MaxUint64 {
					if omv.BucketID == omk.BucketID {
						return omk, nil
					}
				} else {
					return omk, nil
				}
			}
		} else {

			return omk, nil
		}
		i++
	}
}

func (l *LfsService) getOtherBucket(ctx context.Context, omk *tx.ObjMetaKey) (*bucket, error) {
	key := tx.ObjMetaKey{
		UserID:   omk.UserID,
		BucketID: omk.BucketID,
		ObjectID: math.MaxUint64,
	}

	val, ok := l.tagCache.Get(key)
	if ok {
		bu, ok := val.(*bucket)
		if ok {
			return bu, nil
		}
	}

	bopt, err := l.StateGetBucOpt(ctx, omk.UserID, omk.BucketID)
	if err != nil {
		return nil, err
	}

	bmp, err := l.StateGetBucMeta(ctx, omk.UserID, omk.BucketID)
	if err != nil {
		return nil, err
	}

	bu := &bucket{
		BucketInfo: types.BucketInfo{
			BucketOption: *bopt,
			BucketInfo: pb.BucketInfo{
				BucketID: omk.BucketID,
				Name:     bmp.Name,
			},
		},
	}

	l.tagCache.Add(key, bu)

	return bu, nil
}

func (l *LfsService) getOtherObject(ctx context.Context, omk *tx.ObjMetaKey, ename string) (*object, error) {
	tag, err := etag.ToByte(ename)
	if err != nil {
		return nil, err
	}

	key := tx.ObjMetaKey{
		UserID:   omk.UserID,
		BucketID: omk.BucketID,
		ObjectID: omk.ObjectID,
	}

	val, ok := l.tagCache.Get(key)
	if ok {
		obj, ok := val.(*object)
		if ok {
			if !bytes.Equal(tag, obj.ETag) {
				return nil, xerrors.Errorf("uncompatible etag and userID-bucketID-objectID")
			}
		}
	}

	omv, err := l.StateGetObjMeta(ctx, omk.UserID, omk.BucketID, omk.ObjectID)
	if err != nil {
		return nil, err
	}

	if !bytes.Equal(tag, omv.ETag) {
		return nil, xerrors.Errorf("uncompatible etag and userID-bucketID-objectID")
	}

	poi := pb.ObjectInfo{
		ObjectID:    omk.ObjectID,
		BucketID:    omk.BucketID,
		Name:        omv.Name,
		Encryption:  omv.Encrypt,
		UserDefined: make(map[string]string),
	}

	poi.UserDefined["nencryption"] = omv.NEncrypt

	if len(omv.Extra) >= 8 {
		csize := binary.BigEndian.Uint64(omv.Extra[:8])
		poi.UserDefined["etag"] = "cid-" + strconv.FormatUint(csize, 10)
	}

	object := &object{
		ObjectInfo: types.ObjectInfo{
			ObjectInfo: poi,
			Size:       omv.Length,
			ETag:       omv.ETag,
			Parts:      make([]*pb.ObjectPartInfo, 0, 1),
			State:      fmt.Sprintf("user: %d", omk.UserID),
		},
		ops:      make([]uint64, 0, 2),
		deletion: false,
	}

	opi := &pb.ObjectPartInfo{
		Offset: omv.Offset,
		Length: omv.Length,
		ETag:   omv.ETag,
	}

	object.Parts = append(object.Parts, opi)

	l.tagCache.Add(key, object)

	return object, nil
}

func (l *LfsService) downloadOtherObjectByCID(ctx context.Context, cidName string, writer io.Writer, opts types.DownloadObjectOptions) error {
	omk, err := l.getOther(ctx, cidName, opts)
	if err != nil {
		return err
	}

	bu, err := l.getOtherBucket(ctx, omk)
	if err != nil {
		return err
	}

	object, err := l.getOtherObject(ctx, omk, cidName)
	if err != nil {
		return err
	}

	return l.downloadObject(ctx, omk.UserID, bu.BucketInfo, object, writer, opts)
}

func (l *LfsService) addSegLoc(ctx context.Context, userID uint64) error {
	for {
		l.Lock()
		g, ok := l.users[userID]
		if !ok {
			l.Unlock()
			return xerrors.Errorf("no such user: %d", userID)
		}

		if !g.segloc {
			g.segloc = true
			l.Unlock()
			break
		}
		l.Unlock()
		logger.Debug("wait retrieve seg location at: ", userID)
		time.Sleep(time.Second)
	}

	defer func() {
		l.Lock()
		g := l.users[userID]
		g.segloc = false
		l.Unlock()
	}()

	l.RLock()
	g := l.users[userID]
	l.RUnlock()

	pros, err := l.StateGetProsAt(ctx, userID)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	for _, proID := range pros {
		wg.Add(1)
		go func(proID uint64) {
			defer wg.Done()
			key := store.NewKey(pb.MetaType_OrderPayInfoKey, userID, proID)
			lns := new(types.NonceSeq)
			val, err := l.ds.Get(key)
			if err == nil {
				lns.Deserialize(val)
			}

			ns, err := l.StateGetOrderNonce(ctx, userID, proID, math.MaxUint64)
			if err != nil {
				return
			}

			nt := time.Now()
			logger.Debug("retrieve seg location at: ", userID, proID, lns.Nonce, lns.SeqNum, ns.Nonce, ns.SeqNum)

			sid, err := segment.NewSegmentID(g.fsID, 0, 0, 0)
			if err != nil {
				return
			}
			sns := &types.NonceSeq{
				Nonce:  lns.Nonce,
				SeqNum: lns.SeqNum,
			}
			for i := lns.Nonce; i <= ns.Nonce; i++ {
				of, err := l.StateGetOrder(ctx, userID, proID, i)
				if err != nil {
					break
				}
				sns.Nonce = i
				sns.SeqNum = lns.SeqNum // reset

				for j := lns.SeqNum; j <= of.SeqNum; j++ {
					sns.SeqNum = j
					seq, err := l.StateGetOrderSeq(ctx, userID, proID, i, j)
					if err != nil {
						break
					}

					for _, seg := range seq.Segments {
						sid.SetBucketID(seg.BucketID)
						sid.SetChunkID(seg.ChunkID)
						for k := seg.Start; k < seg.Start+seg.Length; k++ {
							sid.SetStripeID(k)

							l.PutSegmentLocation(ctx, sid, seq.ProID)
						}
					}
				}
				lns.SeqNum = 0
			}

			da, err := sns.Serialize()
			if err == nil {
				l.ds.Put(key, da)
			}
			logger.Debug("retrieve seg location end at: ", userID, proID, sns.Nonce, sns.SeqNum, ns.Nonce, ns.SeqNum, time.Since(nt))
		}(proID)
	}
	wg.Wait()

	return nil
}
