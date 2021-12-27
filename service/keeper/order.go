package keeper

import (
	"math/rand"
	"time"
)

func (k *KeeperNode) updateOrder() {
	logger.Debug("start update order")

	rand.NewSource(time.Now().UnixNano())
	t := rand.Intn(60)
	time.Sleep(time.Duration(t) * time.Second)

	ticker := time.NewTicker(3 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-k.ctx.Done():
			logger.Warn("update order context done ", k.ctx.Err())
			return
		case <-ticker.C:
			users := k.GetAllUsers(k.ctx)
			for _, uid := range users {
				k.addOrder(uid)
			}
		}
	}
}

func (k *KeeperNode) addOrder(userID uint64) error {
	logger.Debug("add order for user: ", userID)

	pros := k.GetProsForUser(k.ctx, userID)
	for _, proID := range pros {
		ns := k.GetOrderState(k.ctx, userID, proID)
		nonce, subNonce, err := k.ContractMgr.GetOrderInfo(userID, proID)
		if err != nil {
			logger.Debug("fail to get order info in chain", userID, proID, err)
			continue
		}

		logger.Debugf("user %d pro %d has order %d %d %d", userID, proID, nonce, subNonce, ns.Nonce)
		for i := nonce; i+1 < ns.Nonce; i++ {
			keepers := k.GetAllKeepers(k.ctx)
			nt := time.Now().Unix() / (600)
			// only one do this
			kindex := (int(userID+proID) + int(nt)) % len(keepers)
			if keepers[kindex] != k.RoleID() {
				continue
			}

			curNonce, _, err := k.ContractMgr.GetOrderInfo(userID, proID)
			if err != nil {
				logger.Debug("fail to get order info in chain", userID, proID, err)
				break
			}

			if curNonce > i {
				break
			}

			logger.Debugf("user %d pro %d add order %d %d", userID, proID, curNonce, ns.Nonce)

			// add order here
			of, err := k.GetOrder(userID, proID, i)
			if err != nil {
				logger.Debug("fail to get order info", userID, proID, err)
				break
			}

			ksigns := make([][]byte, 7)
			err = k.ContractMgr.AddOrder(&of.SignedOrder, ksigns)
			if err != nil {
				logger.Debug("fail to add order ", userID, proID, err)
				break
			}

			avail, err := k.ContractMgr.GetBalance(k.ctx, userID)
			if err != nil {
				logger.Debug("fail to get balance ", userID, proID, err)
				break
			}

			logger.Debugf("user %d has balance %d", userID, avail)

			avail, err = k.ContractMgr.GetBalance(k.ctx, proID)
			if err != nil {
				logger.Debug("fail to get balance ", userID, proID, err)
				break
			}

			logger.Debugf("pro %d has balance %d", proID, avail)
		}
	}

	return nil
}
