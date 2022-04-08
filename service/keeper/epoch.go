package keeper

import (
	"context"
	"math/rand"
	"time"

	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
)

func (k *KeeperNode) updateChalEpoch() {
	logger.Debug("start update epoch")

	rand.NewSource(time.Now().UnixNano())
	t := rand.Intn(120)
	time.Sleep(time.Duration(t) * time.Second)

	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-k.ctx.Done():
			return
		case <-ticker.C:
			sgi, err := k.inp.StateGetInfo(k.ctx)
			if err != nil {
				continue
			}

			chalDur, ok := build.ChalDurMap[sgi.Version]
			if !ok {
				continue
			}

			ce, err := k.StateGetChalEpochInfo(k.ctx)
			if err != nil {
				continue
			}

			if ce.Slot < sgi.Slot && sgi.Slot-ce.Slot > chalDur {
				// update
				logger.Debug("update epoch to: ", ce.Epoch, sgi.Epoch, ce.Slot, sgi.Slot)
				ep := tx.SignedEpochParams{
					EpochParams: tx.EpochParams{
						Epoch: sgi.Epoch,
						Prev:  ce.Seed,
					},
					Sig: types.NewMultiSignature(types.SigSecp256k1),
				}

				h := ep.Hash()

				sig, err := k.RoleSign(k.ctx, k.RoleID(), h.Bytes(), types.SigSecp256k1)
				if err != nil {
					continue
				}

				ep.Sig.Add(k.RoleID(), sig)

				data, err := ep.Serialize()
				if err != nil {
					continue
				}

				msg := &tx.Message{
					Version: 0,
					From:    k.RoleID(),
					To:      k.RoleID(),
					Method:  tx.UpdateChalEpoch,
					Params:  data,
				}
				k.pushMsg(msg)
			}
		}
	}
}

func (k *KeeperNode) pushMsg(msg *tx.Message) {
	var mid types.MsgID
	retry := 0
	for retry < 60 {
		retry++
		id, err := k.PushMessage(k.ctx, msg)
		if err != nil {
			time.Sleep(10 * time.Second)
			continue
		}
		mid = id
		break
	}

	go func(mid types.MsgID) {
		ctx, cancle := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancle()
		logger.Debug("waiting tx message done: ", mid)

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			st, err := k.SyncGetTxMsgStatus(ctx, mid)
			if err != nil {
				time.Sleep(5 * time.Second)
				continue
			}

			if st.Status.Err == 0 {
				logger.Debug("tx message done success: ", mid, msg.From, msg.To, msg.Method, st.BlockID, st.Height)
			} else {
				logger.Warn("tx message done fail: ", mid, msg.From, msg.To, msg.Method, st.BlockID, st.Height, st.Status)
			}
			break
		}
	}(mid)
}
