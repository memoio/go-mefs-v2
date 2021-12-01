package challenge

import (
	"context"
	"time"

	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
)

func (s *SegMgr) pushMessage(msg *tx.Message, epoch uint64) {
	var mid types.MsgID
	for {
		id, err := s.PushMessage(s.ctx, msg)
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}
		mid = id
		break
	}

	go func(mid types.MsgID) {
		ctx, cancle := context.WithTimeout(s.ctx, 10*time.Minute)
		defer cancle()
		for {
			st, err := s.GetTxMsgStatus(ctx, mid)
			if err != nil {
				time.Sleep(5 * time.Second)
				continue
			}

			logger.Debug("tx message done: ", mid, st.BlockID, st.Height, st.Status.Err, string(st.Status.Extra))

			s.chalChan <- &chal{
				userID:  msg.To,
				epoch:   epoch,
				errCode: uint16(st.Status.Err),
			}

			break
		}

	}(mid)
}
