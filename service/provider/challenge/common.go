package challenge

import (
	"context"
	"time"

	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"golang.org/x/xerrors"
)

var logger = logging.Logger("pro-challenge")

type chalRes struct {
	userID  uint64
	epoch   uint64
	errCode uint16
}

type subRes struct {
	userID  uint64
	nonce   uint64
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

func (s *SegMgr) pushMessage(msg *tx.Message) {
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
			select {
			case <-ctx.Done():
				return
			default:
			}
			st, err := s.SyncGetTxMsgStatus(ctx, mid)
			if err != nil {
				time.Sleep(5 * time.Second)
				continue
			}

			if st.Status.Err == 0 {
				logger.Debug("tx message done success: ", mid, msg.From, msg.To, msg.Method, st.BlockID, st.Height)
			} else {
				logger.Warn("tx message done fail: ", mid, msg.From, msg.To, msg.Method, st.BlockID, st.Height, st.Status)
			}

			switch msg.Method {
			case tx.SegmentProof:
				scp := new(tx.SegChalParams)
				err = scp.Deserialize(msg.Params)
				if err != nil {
					return
				}

				s.chalChan <- &chalRes{
					userID:  msg.To,
					epoch:   scp.Epoch,
					errCode: uint16(st.Status.Err),
				}
			}

			return
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
				logger.Debug("tx message done success: ", mid, msg.From, msg.To, msg.Method, st.BlockID, st.Height)
			} else {
				logger.Warn("tx message done fail: ", mid, msg.From, msg.To, msg.Method, st.BlockID, st.Height, st.Status)
				return xerrors.Errorf("msg is invalid: %s", st.Status)
			}

			return nil
		}
	}
}
