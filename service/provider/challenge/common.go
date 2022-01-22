package challenge

import (
	"context"
	"time"

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
	//pk     pdpcommon.PublicKey

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
			st, err := s.SyncGetTxMsgStatus(ctx, mid)
			if err != nil {
				time.Sleep(5 * time.Second)
				continue
			}

			if st.Status.Err == 0 {
				logger.Debug("tx message done success: ", mid, st.BlockID, st.Height)
			} else {
				logger.Warn("tx message done fail: ", mid, st.BlockID, st.Height, st.Status)
			}

			s.chalChan <- &chal{
				userID:  msg.To,
				epoch:   epoch,
				errCode: uint16(st.Status.Err),
			}

			break
		}

	}(mid)
}

func (s *SegMgr) pushAndWaitMessage(msg *tx.Message) error {
	ctx, cancle := context.WithTimeout(s.ctx, 10*time.Minute)
	defer cancle()

	var mid types.MsgID
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			id, err := s.PushMessage(s.ctx, msg)
			if err != nil {
				time.Sleep(5 * time.Second)
				continue
			}
			mid = id
		}
		break
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			st, err := s.SyncGetTxMsgStatus(ctx, mid)
			if err != nil {
				time.Sleep(5 * time.Second)
				continue
			}

			if st.Status.Err == 0 {
				logger.Debug("tx message done success: ", mid, st.BlockID, st.Height)
			} else {
				logger.Warn("tx message done fail: ", mid, st.BlockID, st.Height, st.Status)
			}

			return nil
		}
	}
}
