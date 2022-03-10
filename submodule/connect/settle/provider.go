package settle

import (
	"log"
	"memoc/contracts/role"

	"github.com/ethereum/go-ethereum/common"
)

// RegisterProvider called by anyone to register Provider role, befor this, you should pledge in PledgePool
func (cm *ContractMgr) registerProvider(pledgePoolAddr common.Address, index uint64, sign []byte) error {
	client := getClient(cm.endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return err
	}

	log.Println("begin RegisterProvider in Role contract...")

	// txopts.gasPrice参数赋值为nil
	auth, errMA := makeAuth(cm.hexSK, nil, nil)
	if errMA != nil {
		return errMA
	}
	tx, err := roleIns.RegisterProvider(auth, index, sign)
	if err != nil {
		return err
	}
	return checkTx(cm.endPoint, tx, "RegisterProvider")
}

func (cm *ContractMgr) RegisterProvider() error {
	logger.Debug("register provider")

	pledgep, err := cm.getPledgeP() // 申请Provider最少需质押的金额
	if err != nil {
		return err
	}

	ple, err := cm.getBalanceInPPool(cm.roleID, cm.tIndex)
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

	err = cm.registerProvider(cm.ppAddr, cm.roleID, nil)
	if err != nil {
		return err
	}
	return nil
}

func (cm *ContractMgr) addProviderToGroup(pIndex uint64, gIndex uint64, sign []byte) error {
	client := getClient(cm.endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return err
	}

	log.Println("begin AddProviderToGroup in Role contract...")

	auth, err := makeAuth(cm.hexSK, nil, nil)
	if err != nil {
		return err
	}
	tx, err := roleIns.AddProviderToGroup(auth, pIndex, gIndex, sign)
	if err != nil {
		return err
	}

	return checkTx(cm.endPoint, tx, "AddProviderToGroup")
}

func (cm *ContractMgr) AddProviderToGroup(gIndex uint64) error {
	err := cm.addProviderToGroup(cm.roleID, gIndex, nil)
	if err != nil {
		return err
	}

	return nil
}
