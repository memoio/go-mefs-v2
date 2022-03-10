package challenge

import (
	"context"

	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/tx"
)

func (s *SegMgr) RemoveSeg(srp *tx.SegRemoveParas) {
	if srp.ProID != s.localID {
		return
	}

	logger.Debug("remove faulted data in order: ", srp.UserID, srp.Nonce, srp.SeqNum)

	si := s.loadFs(srp.UserID)

	for _, seg := range srp.Segments {
		for i := seg.Start; i < seg.Start+seg.Length; i++ {
			sid, err := segment.NewSegmentID(si.fsID, seg.BucketID, i, seg.ChunkID)
			if err != nil {
				continue
			}
			s.DeleteSegment(context.TODO(), sid)
		}
	}
}
