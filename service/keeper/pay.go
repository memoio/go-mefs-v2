package keeper

import (
	"encoding/binary"
	"math/big"
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

			pros := k.GetAllProviders(k.ctx)
			for _, pid := range pros {
				logger.Debugf("pay for %d at epoch %d", pid, payEpoch)
				users := k.GetUsersForPro(k.ctx, pid)
				if len(users) == 0 {
					logger.Debugf("pay for %d at epoch %d, not have users", pid, payEpoch)
					continue
				}

				pip := &tx.PostIncomeParams{
					Epoch: payEpoch,
					Users: make([]uint64, 0, len(users)),
					Income: types.AccPostIncome{
						ProID:   pid,
						Value:   big.NewInt(0),
						Penalty: big.NewInt(0),
					},
				}

				for _, uid := range users {
					pi, err := k.PushPool.GetPostIncomeAt(k.ctx, uid, pid, payEpoch)
					if err != nil {
						// todo: add penalty here?
						logger.Debugf("pay not have post income for %d %d at epoch %d", uid, pid, payEpoch)
						continue
					}
					if pi == nil || pi.Value == nil || pi.Penalty == nil {
						continue
					}
					if pi.Value.BitLen() == 0 && pi.Penalty.BitLen() == 0 {
						continue
					}

					pip.Income.Value.Add(pip.Income.Value, pi.Value)
					pip.Income.Penalty.Add(pip.Income.Penalty, pi.Penalty)
					pip.Users = append(pip.Users, uid)
				}

				if len(pip.Users) == 0 {
					logger.Debugf("pay not have positive post income for %d at epoch %d", pros, payEpoch)
					continue
				}

				// add base
				spi, err := k.PushPool.GetAccPostIncomeAt(k.ctx, pid, payEpoch)
				if err == nil {
					pip.Income.Value.Add(pip.Income.Value, spi.Value)
					pip.Income.Penalty.Add(pip.Income.Penalty, spi.Penalty)
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
					Method:  tx.PostIncome,
					Params:  data,
				}

				k.pushMsg(msg)
				logger.Debugf("pay for pro %d at epoch %d users %d ", pip.Income.ProID, payEpoch, pip.Users)
			}
			key = store.NewKey(pb.MetaType_ConfirmPayKey)
			binary.BigEndian.PutUint64(buf, latest)
			k.MetaStore().Put(key, buf)
		}
	}
}
