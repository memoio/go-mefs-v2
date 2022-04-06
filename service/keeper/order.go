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
			k.subOrderAll()
		}
	}
}

func (k *KeeperNode) subOrderAll() {
	go func() {
		if k.inProcess {
			return
		}
		k.inProcess = true
		defer func() {
			k.inProcess = false
		}()
		users := k.StateGetAllUsers(k.ctx)
		for _, uid := range users {
			k.subOrder(uid)
		}
	}()
}

func (k *KeeperNode) subOrder(userID uint64) error {
	logger.Debug("subOrder for user: ", userID)

	pros := k.StateGetProsAt(k.ctx, userID)
	for _, proID := range pros {
		keepers := k.StateGetAllKeepers(k.ctx)
		nt := time.Now().Unix() / (600)
		// only one do this
		kindex := (int(userID+proID) + int(nt)) % len(keepers)
		if keepers[kindex] != k.RoleID() {
			continue
		}

		si, err := k.ContractMgr.SettleGetStoreInfo(k.ctx, userID, proID)
		if err != nil {
			logger.Debug("subOrder fail to get order info in chain: ", userID, proID, err)
			continue
		}

		if si.Nonce == si.SubNonce {
			continue
		}

		ns := k.StateGetOrderState(k.ctx, userID, proID)
		logger.Debugf("subOrder user %d pro %d has order %d %d %d", userID, proID, si.Nonce, si.SubNonce, ns.Nonce)

		if si.SubNonce >= ns.Nonce {
			continue
		}

		// sub order here
		of, err := k.StateGetOrder(k.ctx, userID, proID, si.SubNonce)
		if err != nil {
			logger.Debug("subOrder fail to get order info: ", userID, proID, err)
			continue
		}

		if of.End >= time.Now().Unix() {
			logger.Debug("subOrder time not up: ", userID, proID, of.End, time.Now().Unix())
			continue
		}

		err = k.ContractMgr.SubOrder(&of.SignedOrder)
		if err != nil {
			logger.Debug("subOrder fail to sub order: ", userID, proID, err)
			continue
		}

		avail, err := k.ContractMgr.SettleGetBalanceInfo(k.ctx, userID)
		if err != nil {
			logger.Debug("subOrder fail to get balance: ", userID, proID, err)
			continue
		}

		logger.Debugf("subOrder user %d has balance %d", userID, avail)

		avail, err = k.ContractMgr.SettleGetBalanceInfo(k.ctx, proID)
		if err != nil {
			logger.Debug("subOrder fail to get balance: ", userID, proID, err)
			continue
		}

		logger.Debugf("subOrder pro %d has balance %d", proID, avail)
	}

	return nil
}
