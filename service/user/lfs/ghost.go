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
	g, ok := l.users[userID]
	if ok {
		return g, nil
	}

	if userID == l.userID {
		g = &ghost{
			keyset: l.keyset,
			fsID:   l.keyset.VerifyKey().Hash(),
		}

		l.Lock()
		l.users[userID] = g
		l.Unlock()

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

func (l *LfsService) getOther(ctx context.Context, etagName string, opts types.DownloadObjectOptions) (*tx.ObjMetaKey, error) {
	tag, err := etag.ToByte(etagName)
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

	omki, ok := l.tagCache.Get(etagName)
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

		l.tagCache.Add(etagName, omk)

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

func (l *LfsService) downloadOtherObjectByEtag(ctx context.Context, etagName string, writer io.Writer, opts types.DownloadObjectOptions) error {
	omk, err := l.getOther(ctx, etagName, opts)
	if err != nil {
		return err
	}

	bu, err := l.getOtherBucket(ctx, omk)
	if err != nil {
		return err
	}

	object, err := l.getOtherObject(ctx, omk, etagName)
	if err != nil {
		return err
	}

	return l.downloadObject(ctx, omk.UserID, bu.BucketInfo, object, writer, opts)
}

func (l *LfsService) addSegLoc(ctx context.Context, userID uint64) error {
	g, err := l.getGhost(ctx, userID)
	if err != nil {
		return xerrors.Errorf("no such user: %d", userID)
	}

	if g.segloc {
		logger.Warnf("wait retrieve seg location at: ", userID)
		return xerrors.Errorf("%d is retrieving seg location", userID)
	}

	g.segloc = true
	defer func() {
		l.Lock()
		g.segloc = false
		l.Unlock()
	}()

	pros, err := l.StateGetProsAt(ctx, userID)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	for _, pID := range pros {
		wg.Add(1)
		go func(proID uint64) {
			defer wg.Done()
			key := store.NewKey(pb.MetaType_OrderSegLocKey, userID, proID, math.MaxInt)
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

			asq := new(types.AggSegsQueue)

			skey := store.NewKey(pb.MetaType_OrderSegLocKey, userID, proID)
			sval, err := l.ds.Get(skey)
			if err == nil && sval != nil {
				asq.Deserialize(sval)
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
						asq.Push(seg)
					}
					asq.Merge()
				}
				lns.SeqNum = 0
			}

			sda, err := asq.Serialize()
			if err == nil {
				l.ds.Put(skey, sda)
			}

			da, err := sns.Serialize()
			if err == nil {
				l.ds.Put(key, da)
			}
			logger.Debug("retrieve seg location end at: ", userID, proID, sns.Nonce, sns.SeqNum, ns.Nonce, ns.SeqNum, len(sda), time.Since(nt))
		}(pID)
	}
	wg.Wait()

	return nil
}

func (l *LfsService) getSegment(ctx context.Context, userID uint64, segID segment.SegmentID) (segment.Segment, error) {
	seg, err := l.OrderMgr.GetSegmentFromLocal(ctx, segID)
	if err == nil {
		return seg, nil
	}

	pid, err := l.getSegmentLocation(ctx, userID, segID)
	if err != nil {
		return nil, err
	}

	return l.OrderMgr.GetSegmentRemote(ctx, segID, pid)
}

func (l *LfsService) getSegmentLocation(ctx context.Context, userID uint64, segID segment.SegmentID) (uint64, error) {
	if l.userID == userID {
		pid, err := l.OrderMgr.GetSegmentLocation(ctx, segID)
		if err == nil {
			return pid, nil
		}
	}

	bucketID := segID.GetBucketID()
	stripeID := segID.GetStripeID()
	chunkID := segID.GetChunkID()

	pros, err := l.StateGetProsAt(ctx, userID)
	if err != nil {
		return 0, err
	}

	for _, pid := range pros {
		key := store.NewKey(pb.MetaType_OrderSegLocKey, userID, pid)
		val, err := l.ds.Get(key)
		if err != nil {
			continue
		}

		asq := new(types.AggSegsQueue)
		err = asq.Deserialize(val)
		if err != nil {
			continue
		}

		if asq.Has(bucketID, stripeID, chunkID) {
			return pid, nil
		}
	}

	return 0, xerrors.Errorf("no location in meta")
}
