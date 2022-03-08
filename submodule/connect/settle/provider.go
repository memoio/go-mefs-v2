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
		err = cm.pledge(pledgep)
		if err != nil {
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
		logger.Fatal("add provider to group fail: ", cm.roleID, gIndex, err)
		return err
	}

	return nil
}
