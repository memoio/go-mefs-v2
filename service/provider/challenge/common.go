package challenge

import (
	"context"
	"time"

	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
)

var logger = logging.Logger("pro-challenge")

type chal struct {
	userID  uint64
	epoch   uint64
	errCode uint16
}

type segInfo struct {
	userID uint64
	fsID   []byte
	pk     pdpcommon.PublicKey

	nextChal uint64
	wait     bool
	chalTime time.Time
}

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
