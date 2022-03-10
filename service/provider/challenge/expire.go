package challenge

import (
	"context"

	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/tx"
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
		of, err := s.GetOrder(userID, s.localID, i)
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

	si := s.loadFs(userID)
	seqNum := uint32(0)
	for {
		sf, err := s.GetOrderSeq(userID, s.localID, nonce, seqNum)
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

		seqNum++
	}
}
