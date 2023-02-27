package lfs

import (
	"context"
	"time"

	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
)

func (l *LfsService) runPush() {
	for {
		select {
		case <-l.ctx.Done():
			return
		case msg := <-l.msgChan:
			l.pushMessage(msg)
		}
	}
}

func (l *LfsService) pushMessage(msg *tx.Message) {
	var mid types.MsgID
	for {
		id, err := l.PushMessage(l.ctx, msg)
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}
		mid = id
		break
	}

	go func(mid types.MsgID) {
		ctx, cancle := context.WithTimeout(l.ctx, 60*time.Minute)
		defer cancle()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			st, err := l.SyncGetTxMsgStatus(ctx, mid)
			if err != nil {
				time.Sleep(5 * time.Second)
				continue
			}

			if st.Status.Err == 0 {
				logger.Debug("tx message done success: ", mid, msg.From, msg.To, msg.Method, st.BlockID, st.Height)
				switch msg.Method {
				case tx.CreateFs:
					l.readyChan <- struct{}{}
				case tx.CreateBucket:
					tbp := new(tx.BucketParams)
					err := tbp.Deserialize(msg.Params)
					if err == nil {
						l.bucketChan <- tbp.BucketID
					}
				default:
				}
			} else {
				logger.Warn("tx message done fail: ", mid, msg.From, msg.To, msg.Method, st.BlockID, st.Height, st.Status)
			}

			break
		}
	}(mid)
}
