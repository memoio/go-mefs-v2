package challenge

import (
	"context"
	"encoding/binary"

	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

func (s *SegMgr) subDataOrder(userID uint64) error {
	logger.Debug("handle expired order for user: ", userID)

	pce, err := s.StateGetChalEpochInfoAt(s.ctx, s.epoch-2)
	if err != nil {
		return err
	}

	target := build.BaseTime + int64(pce.Slot*build.SlotDuration)

	ns := s.StateGetOrderState(s.ctx, userID, s.localID)
	for i := ns.SubNonce; i <= ns.Nonce; i++ {
		of, err := s.StateGetOrder(s.ctx, userID, s.localID, i)
		if err != nil {
			return err
		}

		if of.End > target {
			return nil
		}

		osp := &tx.OrderSubParas{
			UserID: userID,
			ProID:  s.localID,
			Nonce:  i,
		}

		data, err := osp.Serialize()
		if err != nil {
			return err
		}

		// submit proof
		msg := &tx.Message{
			Version: 0,
			From:    s.localID,
			To:      userID,
			Method:  tx.SubDataOrder,
			Params:  data,
		}

		s.pushMessage(msg)
	}

	return nil
}

func (s *SegMgr) removeSegInExpiredOrder(userID, nonce uint64) error {
	logger.Debug("remove data in expired order: ", userID, nonce)

	of, err := s.StateGetOrder(s.ctx, userID, s.localID, nonce)
	if err != nil {
		return err
	}

	si := s.loadFs(userID)
	for i := uint32(0); i < of.SeqNum; i++ {
		sf, err := s.StateGetOrderSeq(s.ctx, userID, s.localID, nonce, i)
		if err != nil {
			return err
		}

		for _, seg := range sf.Segments {
			for i := seg.Start; i < seg.Start+seg.Length; i++ {
				sid, err := segment.NewSegmentID(si.fsID, seg.BucketID, i, seg.ChunkID)
				if err != nil {
					continue
				}
				s.DeleteSegment(context.TODO(), sid)
			}
		}
	}

	key := store.NewKey(pb.MetaType_OrderExpiredKey, userID, s.localID)
	val := make([]byte, 8)
	binary.BigEndian.PutUint64(val, nonce+1)
	return s.ds.Put(key, val)
}
