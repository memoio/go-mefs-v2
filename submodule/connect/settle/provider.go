package settle

import (
	"log"
)

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
			log.Fatal(err)
		}
	}

	logger.Debugf("provider register %d", pledgep)

	return cm.iRole.RegisterProvider(cm.ppAddr, cm.roleID, nil)
}

func (cm *ContractMgr) AddProviderToGroup(gIndex uint64) error {
	return cm.iRole.AddProviderToGroup(cm.roleID, gIndex, nil)
}
