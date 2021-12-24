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
			// todo: should get from state
			key := store.NewKey(pb.MetaType_Chal_UsersKey)
			data, err := k.MetaStore().Get(key)
			if err != nil {
				logger.Debugf("pay no users at epoch %d", payEpoch)
				continue
			}

			users := make([]uint64, len(data)/8)
			for i := 0; i < len(data)/8; i++ {
				users[i] = binary.BigEndian.Uint64(data[8*i : 8*(i+1)])
			}

			for _, uid := range users {
				key := store.NewKey(pb.MetaType_Chal_ProsKey, uid)
				data, err := k.MetaStore().Get(key)
				if err != nil {
					logger.Debugf("pay no pros for user %d at epoch %d", uid, payEpoch)
					continue
				}

				pros := make([]uint64, len(data)/8)
				for i := 0; i < len(data)/8; i++ {
					pros[i] = binary.BigEndian.Uint64(data[8*i : 8*(i+1)])
				}

				pip := &tx.PostIncomeParams{
					Epoch:  payEpoch,
					UserID: uid,
					Pros:   make([]uint64, 0, len(pros)),
					Sig:    make([]types.Signature, 0, len(pros)),
				}

				for _, pid := range pros {
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
					sig, err := k.RoleSign(k.ctx, k.RoleID(), pi.Hash(), types.SigSecp256k1)
					if err != nil {
						continue
					}
					pip.Pros = append(pip.Pros, pid)
					pip.Sig = append(pip.Sig, sig)
				}

				if len(pip.Pros) == 0 {
					logger.Debugf("pay not have positive post income for %d %d at epoch %d", uid, pros, payEpoch)
					continue
				}

				data, err = pip.Serialize()
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
				logger.Debugf("pay for user %d at epoch %d pro %d ", pip.UserID, payEpoch, pip.Pros)
			}
			key = store.NewKey(pb.MetaType_ConfirmPayKey)
			binary.BigEndian.PutUint64(buf, latest)
			k.MetaStore().Put(key, buf)
		}
	}
}
