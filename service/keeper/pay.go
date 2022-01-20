package keeper

import (
	"encoding/binary"
	"time"

	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

func (k *KeeperNode) updatePay() {
	logger.Debug("start update pay")
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	latest := k.PushPool.GetChalEpoch(k.ctx)

	key := store.NewKey(pb.MetaType_ConfirmPayKey)
	val, err := k.MetaStore().Get(key)
	if err == nil && len(val) >= 8 {
		latest = binary.BigEndian.Uint64(val)
	}

	buf := make([]byte, 8)

	for {
		select {
		case <-k.ctx.Done():
			logger.Warn("pay context done ", k.ctx.Err())
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

			logger.Debugf("pay at epoch %d", payEpoch)

			pros := k.StateGetAllProviders(k.ctx)
			for _, pid := range pros {
				logger.Debugf("pay for %d at epoch %d", pid, payEpoch)
				users := k.StateGetUsersAt(k.ctx, pid)
				if len(users) == 0 {
					logger.Debugf("pay for %d at epoch %d, not have users", pid, payEpoch)
					continue
				}

				spi, err := k.PushPool.StateGetAccPostIncomeAt(k.ctx, pid, payEpoch)
				if err != nil {
					logger.Debugf("pay for %d at epoch %d, not have challenge or declare fault", pid, payEpoch)
					continue
				}

				pip := &tx.PostIncomeParams{
					Epoch:  payEpoch,
					Income: *spi,
				}

				sig, err := k.RoleSign(k.ctx, k.RoleID(), pip.Income.Hash(), types.SigSecp256k1)
				if err != nil {
					continue
				}
				pip.Sig = sig

				data, err := pip.Serialize()
				if err != nil {
					continue
				}

				msg := &tx.Message{
					Version: 0,
					From:    k.RoleID(),
					To:      k.RoleID(),
					Method:  tx.ConfirmPostIncome,
					Params:  data,
				}

				k.pushMsg(msg)
				logger.Debugf("pay for pro %d at epoch %d users %d ", pip.Income.ProID, payEpoch, users)
			}
			key = store.NewKey(pb.MetaType_ConfirmPayKey)
			binary.BigEndian.PutUint64(buf, latest)
			k.MetaStore().Put(key, buf)
		}
	}
}
