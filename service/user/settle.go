package user

import (
	"math/big"
	"time"
)

func (u *UserNode) recharge() {
	logger.Debug("start update order")

	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-u.ctx.Done():
			logger.Warn("recharge context done ", u.ctx.Err())
			return
		case <-ticker.C:
			u.charge()
			u.addOrder()
		}
	}
}

func (u *UserNode) charge() error {
	userID := u.RoleID()
	logger.Debug("charge for user: ", userID)
	pros := u.GetProsForUser(u.ctx, userID)

	totalPay := new(big.Int)
	for _, proID := range pros {
		ns := u.GetOrderState(u.ctx, userID, proID)
		nonce, subNonce, err := u.ContractMgr.GetOrderInfo(userID, proID)
		if err != nil {
			logger.Debug("fail to get order info in chain", userID, proID, err)
			continue
		}

		logger.Debugf("user %d pro %d has order %d %d %d", userID, proID, nonce, subNonce, ns.Nonce)
		for i := nonce; i < ns.Nonce; i++ {
			// add order here
			of, err := u.GetOrder(userID, proID, i)
			if err != nil {
				logger.Debug("fail to get order info", userID, proID, err)
				continue
			}
			of.Price.Mul(of.Price, big.NewInt(of.End-of.Start))
			totalPay.Add(totalPay, of.Price)
		}
	}

	totalPay.Mul(totalPay, big.NewInt(12))
	totalPay.Div(totalPay, big.NewInt(10))

	return u.ContractMgr.Recharge(totalPay)
}

func (u *UserNode) addOrder() error {
	userID := u.RoleID()
	logger.Debug("add order for user: ", userID)

	pros := u.GetProsForUser(u.ctx, userID)
	for _, proID := range pros {
		ns := u.GetOrderState(u.ctx, userID, proID)
		nonce, subNonce, err := u.ContractMgr.GetOrderInfo(userID, proID)
		if err != nil {
			logger.Debug("fail to get order info in chain", userID, proID, err)
			continue
		}

		logger.Debugf("user %d pro %d has order %d %d %d", userID, proID, nonce, subNonce, ns.Nonce)
		for i := nonce; i+1 < ns.Nonce; i++ {
			logger.Debugf("user %d pro %d add order %d %d", userID, proID, i, ns.Nonce)

			// add order here
			of, err := u.GetOrder(userID, proID, i)
			if err != nil {
				logger.Debug("fail to get order info", userID, proID, err)
				break
			}

			ksigns := make([][]byte, 7)
			err = u.ContractMgr.AddOrder(&of.SignedOrder, ksigns)
			if err != nil {
				logger.Debug("fail to add order ", userID, proID, err)
				break
			}

			avail, err := u.ContractMgr.GetBalance(u.ctx, userID)
			if err != nil {
				logger.Debug("fail to get balance ", userID, proID, err)
				break
			}

			logger.Debugf("user %d has balance %d", userID, avail)

			avail, err = u.ContractMgr.GetBalance(u.ctx, proID)
			if err != nil {
				logger.Debug("fail to get balance ", userID, proID, err)
				break
			}

			logger.Debugf("pro %d has balance %d", proID, avail)
		}
	}

	return nil
}
