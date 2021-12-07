package keeper

import (
	"time"

	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
)

func (k *KeeperNode) updatePay() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	latest := k.PushPool.GetChalEpoch(k.ctx)

	for {
		select {
		case <-k.ctx.Done():
			return
		case <-ticker.C:
			cur := k.PushPool.GetChalEpoch(k.ctx)
			if cur <= latest {
				continue
			}

			latest = cur

			if latest < 2 {
				continue
			}

			payEpoch := latest - 2

			users := k.PushPool.GetAllUsers(k.ctx)
			for _, uid := range users {
				pros := k.PushPool.GetProsForUser(k.ctx, uid)
				pip := &tx.PostIncomeParams{
					Epoch:  payEpoch,
					UserID: uid,
					Pros:   make([]uint64, 0, len(pros)),
					Sig:    make([]types.Signature, 0, len(pros)),
				}

				for _, pid := range pros {
					pi := k.PushPool.GetPostIncomeAt(k.ctx, uid, pid, payEpoch)
					if pi == nil || pi.Value == nil || pi.Penalty == nil {
						continue
					}
					if pi.Value.BitLen() == 0 && pi.Penalty.BitLen() == 0 {
						continue
					}
					sig, err := k.RoleSign(k.ctx, k.RoleID(), pi.Hash(), types.SigSecp256k1)
					if err != nil {
						continue
					}

					pip.Pros = append(pip.Pros, pid)
					pip.Sig = append(pip.Sig, sig)
				}

				data, err := pip.Serialize()
				if err != nil {
					continue
				}

				msg := &tx.Message{
					Version: 0,
					From:    k.RoleID(),
					To:      k.RoleID(),
					Method:  tx.PostIncome,
					Params:  data,
				}

				k.pushMsg(msg)
			}
		}
	}
}
