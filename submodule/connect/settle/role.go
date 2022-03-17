package settle

import (
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/xerrors"

	callconts "memoc/callcontracts"
	"memoc/contracts/role"

	"github.com/memoio/go-mefs-v2/lib/pb"
)

// GetAddrsNum get the number of registered addresses.
func (cm *ContractMgr) getAddrCount() (uint64, error) {
	var anum uint64

	client := getClient(cm.endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return anum, err
	}

	retryCount := 0
	for {
		retryCount++
		anum, err = roleIns.GetAddrsNum(&bind.CallOpts{
			From: cm.eAddr,
		})
		if err != nil {
			if retryCount > sendTransactionRetryCount {
				return anum, err
			}
			time.Sleep(retryGetInfoSleepTime)
			continue
		}

		return anum, nil
	}
}

func (cm *ContractMgr) getAddrAt(rIndex uint64) (common.Address, error) {
	var addr common.Address

	client := getClient(cm.endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return addr, err
	}

	retryCount := 0
	if rIndex == 0 {
		return addr, xerrors.New("roleIndex should not be 0")
	}

	sum, err := cm.getAddrCount()
	if err != nil {
		return addr, err
	}

	if rIndex > sum {
		return addr, xerrors.Errorf("roleIndex %d is larger than %d", rIndex, sum)
	}

	rIndex-- // get address by array index actually in contract, which is rIndex minus 1

	for {
		retryCount++
		addr, err = roleIns.GetAddr(&bind.CallOpts{
			From: cm.eAddr,
		}, rIndex)
		if err != nil {
			if retryCount > sendTransactionRetryCount {
				return addr, err
			}
			time.Sleep(retryGetInfoSleepTime)
			continue
		}

		return addr, nil
	}
}

func (cm *ContractMgr) getRoleInfo(addr common.Address) (*roleInfo, error) {
	client := getClient(cm.endPoint)
	defer client.Close()

	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return nil, err
	}

	ri := &roleInfo{
		pri: new(pb.RoleInfo),
	}

	retryCount := 0
	for {
		retryCount++
		isActive, isBanned, rType, rid, gid, extra, err := roleIns.GetRoleInfo(&bind.CallOpts{
			From: cm.eAddr,
		}, addr)
		if err != nil {
			if retryCount > sendTransactionRetryCount {
				return nil, err
			}
			time.Sleep(retryGetInfoSleepTime)
			continue
		}

		ri.isActive = isActive
		ri.isBanned = isBanned
		ri.pri.RoleID = rid
		ri.pri.GroupID = gid
		ri.pri.ChainVerifyKey = addr.Bytes()

		switch rType {
		case callconts.UserRoleType:
			ri.pri.Type = pb.RoleInfo_User
			ri.pri.Extra = extra
		case callconts.ProviderRoleType:
			ri.pri.Type = pb.RoleInfo_Provider
		case callconts.KeeperRoleType:
			ri.pri.Type = pb.RoleInfo_Keeper
			ri.pri.BlsVerifyKey = extra
		default:
			ri.pri.Type = pb.RoleInfo_Unknown
		}

		return ri, nil
	}
}
