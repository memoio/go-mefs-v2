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
				//k.addOrder(uid)
				k.subOrder(uid)
			}
		}
	}
}

func (k *KeeperNode) addOrder(userID uint64) error {
	logger.Debug("addOrder for user: ", userID)

	pros := k.GetProsForUser(k.ctx, userID)
	for _, proID := range pros {
		ns := k.GetOrderState(k.ctx, userID, proID)
		nonce, subNonce, err := k.ContractMgr.GetOrderInfo(userID, proID)
		if err != nil {
			logger.Debug("addOrder fail to get order info in chain", userID, proID, err)
			continue
		}

		logger.Debugf("addOrder user %d pro %d has order %d %d %d", userID, proID, nonce, subNonce, ns.Nonce)
		for i := nonce; i < ns.Nonce; i++ {
			keepers := k.GetAllKeepers(k.ctx)
			nt := time.Now().Unix() / (600)
			// only one do this
			kindex := (int(userID+proID) + int(nt)) % len(keepers)
			if keepers[kindex] != k.RoleID() {
				continue
			}

			curNonce, _, err := k.ContractMgr.GetOrderInfo(userID, proID)
			if err != nil {
				logger.Debug("addOrder fail to get order info in chain", userID, proID, err)
				break
			}

			if curNonce > i {
				break
			}

			logger.Debugf("addOrder user %d pro %d nonce %d", userID, proID, i)

			// add order here
			of, err := k.GetOrder(userID, proID, i)
			if err != nil {
				logger.Debug("addOrder fail to get order info", userID, proID, err)
				break
			}

			ksigns := make([][]byte, 7)
			err = k.ContractMgr.AddOrder(&of.SignedOrder, ksigns)
			if err != nil {
				logger.Debug("addOrder fail to add order ", userID, proID, err)
				break
			}

			avail, err := k.ContractMgr.GetBalance(k.ctx, userID)
			if err != nil {
				logger.Debug("addOrder fail to get balance ", userID, proID, err)
				break
			}

			logger.Debugf("addOrder user %d has balance %d", userID, avail)

			avail, err = k.ContractMgr.GetBalance(k.ctx, proID)
			if err != nil {
				logger.Debug("addOrder fail to get balance ", userID, proID, err)
				break
			}

			logger.Debugf("addOrder pro %d has balance %d", proID, avail)
		}
	}

	return nil
}

func (k *KeeperNode) subOrder(userID uint64) error {
	logger.Debug("subOrder for user: ", userID)

	pros := k.GetProsForUser(k.ctx, userID)
	for _, proID := range pros {
		keepers := k.GetAllKeepers(k.ctx)
		nt := time.Now().Unix() / (600)
		// only one do this
		kindex := (int(userID+proID) + int(nt)) % len(keepers)
		if keepers[kindex] != k.RoleID() {
			continue
		}

		nonce, subNonce, err := k.ContractMgr.GetOrderInfo(userID, proID)
		if err != nil {
			logger.Debug("subOrder fail to get order info in chain: ", userID, proID, err)
			continue
		}

		if nonce == subNonce {
			continue
		}

		ns := k.GetOrderState(k.ctx, userID, proID)
		logger.Debugf("subOrder user %d pro %d has order %d %d %d", userID, proID, nonce, subNonce, ns.Nonce)

		if subNonce >= ns.Nonce {
			continue
		}

		// sub order here
		of, err := k.GetOrder(userID, proID, subNonce)
		if err != nil {
			logger.Debug("subOrder fail to get order info: ", userID, proID, err)
			continue
		}

		if of.End >= time.Now().Unix() {
			logger.Debug("subOrder time not up: ", userID, proID, of.End, time.Now().Unix())
			continue
		}

		ksigns := make([][]byte, 7)
		err = k.ContractMgr.SubOrder(&of.SignedOrder, ksigns)
		if err != nil {
			logger.Debug("subOrder fail to sub order: ", userID, proID, err)
			continue
		}

		avail, err := k.ContractMgr.GetBalance(k.ctx, userID)
		if err != nil {
			logger.Debug("subOrder fail to get balance: ", userID, proID, err)
			continue
		}

		logger.Debugf("subOrder user %d has balance %d", userID, avail)

		avail, err = k.ContractMgr.GetBalance(k.ctx, proID)
		if err != nil {
			logger.Debug("subOrder fail to get balance: ", userID, proID, err)
			continue
		}

		logger.Debugf("subOrder pro %d has balance %d", proID, avail)
	}

	return nil
}
