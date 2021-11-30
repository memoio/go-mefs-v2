package keeper

import (
	"context"
	"time"

	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
)

func (k *KeeperNode) updateEpoch() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-k.ctx.Done():
			return
		case <-ticker.C:
			h := k.StateDB.GetHeight()
			ne := k.StateDB.GetEpoch()
			ce := k.StateDB.GetChalEpoch()
			if ce.Height < h && h-ce.Height > build.DefaultChalDuration {
				// update
				logger.Debug("update epoch to: ", ce.Epoch, ne, ce.Height, h)
				ep := tx.SignedEpochParams{
					EpochParams: tx.EpochParams{
						Epoch: ne,
						Prev:  ce.Seed,
					},
					Sig: types.NewMultiSignature(types.SigSecp256k1),
				}

				h, err := ep.Hash()
				if err != nil {
					continue
				}

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
					Method:  tx.UpdateEpoch,
					Params:  data,
				}

				// handle result and retry?

				var mid types.MsgID
				retry := 0
				for retry < 60 {
					retry++
					id, err := k.PPool.PushMessage(k.ctx, msg)
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
						st, err := k.PPool.GetTxMsgStatus(ctx, mid)
						if err != nil {
							time.Sleep(5 * time.Second)
							continue
						}

						logger.Debug("tx message done: ", mid, st.BlockID, st.Height, st.Status.Err, string(st.Status.Extra))
						break
					}
				}(mid)
			}
		}
	}
}
