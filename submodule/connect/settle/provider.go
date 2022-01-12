package settle

func (cm *ContractMgr) RegisterProvider() error {
	logger.Debug("register provider")

	pledgep, err := cm.iRole.PledgeP() // 申请Provider最少需质押的金额
	if err != nil {
		return err
	}

	ple, err := cm.iPP.GetBalanceInPPool(cm.roleID, cm.tIndex)
	if err != nil {
		return err
	}

	if ple.Cmp(pledgep) < 0 {
		bal, err := cm.iErc.BalanceOf(cm.eAddr)
		if err != nil {
			logger.Debug(err)
			return err
		}

		logger.Debugf("erc20 balance is %d", bal)

		if bal.Cmp(pledgep) < 0 {
			erc20Transfer(cm.eAddr, pledgep)
		}

		logger.Debugf("provider pledge %d", pledgep)

		err = cm.iPP.Pledge(cm.tAddr, cm.rAddr, cm.roleID, pledgep, nil)
		if err != nil {
			return err
		}

		if err = <-cm.status; err != nil {
			logger.Fatal("provider pledge fail: ", err)
			return err
		}

	}

	logger.Debugf("provider register %d", pledgep)

	err = cm.iRole.RegisterProvider(cm.ppAddr, cm.roleID, nil)
	if err != nil {
		return err
	}

	if err = <-cm.status; err != nil {
		logger.Fatal("register pro fail: ", cm.roleID, err)
		return err
	}
	return nil
}

func (cm *ContractMgr) AddProviderToGroup(gIndex uint64) error {
	err := cm.iRole.AddProviderToGroup(cm.roleID, gIndex, nil)
	if err != nil {
		return err
	}

	if err = <-cm.status; err != nil {
		logger.Fatal("add keeper to group fail: ", cm.roleID, gIndex, err)
		return err
	}

	return nil
}
