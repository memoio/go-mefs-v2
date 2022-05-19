package settle

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"math/big"
	"time"

	"memoc/contracts/role"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/pb"
)

var _ api.ISettle = &ContractMgr{}

type ContractMgr struct {
	ctx context.Context

	endPoint string
	chainID  *big.Int

	hexSK   string
	roleID  uint64
	groupID uint64
	level   int
	tIndex  uint32

	eAddr   common.Address // local address
	rAddr   common.Address // role contract address
	rtAddr  common.Address // token mgr address
	tAddr   common.Address // token address
	ppAddr  common.Address // pledge pool address
	isAddr  common.Address // issurance address
	rfsAddr common.Address // issurance address
	fsAddr  common.Address // fs contract addr
}

func NewContractMgr(ctx context.Context, endPoint, roleAddr string, sk []byte) (*ContractMgr, error) {
	logger.Debug("create contract mgr: ", endPoint, ", ", roleAddr)

	client := getClient(endPoint)
	chainID, err := client.NetworkID(context.Background())
	if err != nil {
		return nil, xerrors.Errorf("eth get networkID from %s fail: %s", endPoint, err)
	}

	// convert key
	hexSk := hex.EncodeToString(sk)
	privateKey, err := crypto.HexToECDSA(hexSk)
	if err != nil {
		return nil, err
	}

	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, xerrors.Errorf("error casting public key to ECDSA")
	}

	eAddr := crypto.PubkeyToAddress(*publicKeyECDSA)

	// transfer eth
	// todo: remove at mainnet
	val := getBalance(endPoint, eAddr)
	logger.Debugf("%s has val %d", eAddr, val)
	if val.BitLen() == 0 {
		return nil, xerrors.Errorf("not have tx fee on chain")
	}

	rAddr := common.HexToAddress(roleAddr)
	logger.Debug("role contract address: ", rAddr.Hex())

	tIndex := uint32(0)
	cm := &ContractMgr{
		ctx:      ctx,
		endPoint: endPoint,
		chainID:  chainID,

		hexSK:  hexSk,
		tIndex: tIndex,

		eAddr: eAddr,
		rAddr: rAddr,
	}

	rtAddr, err := GetRoleTokenAddr(cm.endPoint, cm.rAddr, cm.eAddr)
	if err != nil {
		return nil, err
	}
	cm.rtAddr = rtAddr
	logger.Debug("token mgr contract address: ", rtAddr.Hex())

	tAddr, err := GetTokenAddr(cm.endPoint, cm.rtAddr, cm.eAddr, cm.tIndex)
	if err != nil {
		return nil, err
	}
	cm.tAddr = tAddr
	logger.Debug("token contract address: ", tAddr.Hex())

	val = cm.getBalanceInErc(eAddr)
	logger.Debugf("%s has: %d (atto Memo)", eAddr, val)

	//if val.BitLen() == 0 {
	//	return nil, xerrors.Errorf("not have erc20 token")
	//}

	ppAddr, err := GetPledgeAddr(cm.endPoint, cm.rAddr, cm.eAddr)
	if err != nil {
		return nil, err
	}
	cm.ppAddr = ppAddr
	logger.Debug("pledge contract address: ", ppAddr.Hex())

	rfsAddr, err := GetRolefsAddr(cm.endPoint, cm.rAddr, cm.eAddr)
	if err != nil {
		return nil, err
	}
	cm.rfsAddr = rfsAddr
	logger.Debug("role fs contract address: ", rfsAddr.Hex())

	isAddr, err := GetIssuanceAddr(cm.endPoint, cm.rAddr, cm.eAddr)
	if err != nil {
		return nil, err
	}

	cm.isAddr = isAddr
	logger.Debug("issu contract address: ", isAddr.Hex())

	return cm, nil
}

func (cm *ContractMgr) Start(typ pb.RoleInfo_Type, gIndex uint64) error {
	logger.Debug("start contract mgr: ", typ, gIndex)
	ri, err := cm.getRoleInfo(cm.eAddr)
	if err != nil {
		return err
	}

	if gIndex == 0 && ri.pri.GroupID == 0 {
		return xerrors.Errorf("group should be larger than zero")
	}

	logger.Debug("get roleinfo: ", ri.pri, ri.isActive, ri.isBanned)

	// register account
	if ri.pri.RoleID == 0 {
		err := cm.RegisterAcc()
		if err != nil {
			return err
		}

		time.Sleep(10 * time.Second)
	}

	ri, err = cm.getRoleInfo(cm.eAddr)
	if err != nil {
		return err
	}

	logger.Debug("get roleinfo after register account: ", ri.pri, ri.isActive, ri.isBanned)

	if ri.pri.RoleID == 0 {
		return xerrors.Errorf("register fails")
	}

	cm.roleID = ri.pri.RoleID

	// register role
	if ri.pri.Type == pb.RoleInfo_Unknown {
		switch typ {
		case pb.RoleInfo_Keeper:
			err = cm.RegisterKeeper()
			if err != nil {
				logger.Debug("register keeper fail: ", err)
				return err
			}
		case pb.RoleInfo_Provider:
			// provider: register,register; add to group
			err = cm.RegisterProvider()
			if err != nil {
				logger.Debug("register provider fail: ", err)
				return err
			}
		case pb.RoleInfo_User:
			// user: resgister user
			// get extra byte
			err = cm.RegisterUser(gIndex)
			if err != nil {
				logger.Debug("register user fail: ", err)
				return err
			}
		}
	}

	time.Sleep(5 * time.Second)

	ri, err = cm.getRoleInfo(cm.eAddr)
	if err != nil {
		return err
	}

	logger.Debug("get roleinfo after register role: ", ri.pri, ri.isActive, ri.isBanned)

	switch typ {
	case pb.RoleInfo_Keeper:
		if ri.pri.Type != typ {
			return xerrors.Errorf("role type wrong, expected %s, got %s", pb.RoleInfo_Keeper, ri.pri.Type)
		}

		if ri.pri.GroupID == 0 && gIndex > 0 {
			return xerrors.Errorf("need grant to add keeper %d to group %d", ri.pri.RoleID, gIndex)
			/*
				err = AddKeeperToGroup(cm.roleID, gIndex)
				if err != nil {
					return err
				}

				time.Sleep(5 * time.Second)

				_, _, rType, _, gid, _, err = cm.iRole.GetRoleInfo(cm.eAddr)
				if err != nil {
					return err
				}

				if gid != gIndex {
					return xerrors.Errorf("group is wrong, expected %d, got %d", gIndex, gid)
				}
			*/
		}

	case pb.RoleInfo_Provider:
		if ri.pri.Type != typ {
			return xerrors.Errorf("role type wrong, expected %s, got %s", pb.RoleInfo_Provider, ri.pri.Type)
		}

		if ri.pri.GroupID == 0 && gIndex > 0 {
			err = cm.AddProviderToGroup(gIndex)
			if err != nil {
				return err
			}

			time.Sleep(5 * time.Second)

			ri, err = cm.getRoleInfo(cm.eAddr)
			if err != nil {
				return err
			}

			if ri.pri.GroupID != gIndex {
				return xerrors.Errorf("group add wrong, expected %d, got %d", gIndex, ri.pri.GroupID)
			}
		}
	case pb.RoleInfo_User:
		if ri.pri.Type != typ {
			return xerrors.Errorf("role type wrong, expected %s, got %s", pb.RoleInfo_User, ri.pri.Type)
		}
	default:
	}

	cm.roleID = ri.pri.RoleID

	if ri.pri.GroupID == 0 {
		cm.groupID = gIndex
	} else {
		cm.groupID = ri.pri.GroupID
	}

	if cm.groupID > 0 {
		if cm.groupID != gIndex && gIndex > 0 {
			return xerrors.Errorf("group is wrong, expected %d, got %d", ri.pri.GroupID, gIndex)
		}

		gi, err := cm.SettleGetGroupInfoAt(cm.ctx, cm.groupID)
		if err != nil {
			return err
		}
		cm.fsAddr = common.HexToAddress(gi.FsAddr)
		cm.level = int(gi.Level)
		logger.Debug("fs contract address: ", cm.fsAddr.Hex())
	}

	return nil
}

func GetRoleTokenAddr(endPoint string, rAddr, addr common.Address) (common.Address, error) {
	var rt common.Address

	client := getClient(endPoint)
	defer client.Close()

	roleIns, err := role.NewRole(rAddr, client)
	if err != nil {
		return rt, err
	}

	retryCount := 0
	for {
		retryCount++
		rt, err = roleIns.RToken(&bind.CallOpts{
			From: addr,
		})
		if err != nil {
			if retryCount > sendTransactionRetryCount {
				return rt, err
			}
			time.Sleep(retryGetInfoSleepTime)
			continue
		}

		return rt, nil
	}
}

func GetTokenAddr(endPoint string, rtAddr, addr common.Address, tIndex uint32) (common.Address, error) {
	var taddr common.Address

	client := getClient(endPoint)
	defer client.Close()
	rToken, err := role.NewRToken(rtAddr, client)
	if err != nil {
		return taddr, err
	}

	retryCount := 0
	for {
		retryCount++
		taddr, err = rToken.GetTA(&bind.CallOpts{
			From: addr,
		}, tIndex)
		if err != nil {
			if retryCount > sendTransactionRetryCount {
				return taddr, err
			}
			time.Sleep(retryGetInfoSleepTime)
			continue
		}

		return taddr, nil
	}
}

func GetPledgeAddr(endPoint string, rAddr, addr common.Address) (common.Address, error) {
	var pp common.Address
	client := getClient(endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(rAddr, client)
	if err != nil {
		return pp, err
	}

	retryCount := 0
	for {
		retryCount++
		pp, err = roleIns.PledgePool(&bind.CallOpts{
			From: addr,
		})
		if err != nil {
			if retryCount > sendTransactionRetryCount {
				return pp, err
			}
			time.Sleep(retryGetInfoSleepTime)
			continue
		}

		return pp, nil
	}
}

func GetRolefsAddr(endPoint string, rAddr, addr common.Address) (common.Address, error) {
	var rfs common.Address

	client := getClient(endPoint)
	defer client.Close()

	roleIns, err := role.NewRole(rAddr, client)
	if err != nil {
		return rfs, err
	}

	retryCount := 0
	for {
		retryCount++
		rfs, err = roleIns.Rolefs(&bind.CallOpts{
			From: addr,
		})
		if err != nil {
			if retryCount > sendTransactionRetryCount {
				return rfs, err
			}
			time.Sleep(retryGetInfoSleepTime)
			continue
		}

		return rfs, nil
	}
}

func GetIssuanceAddr(endPoint string, rAddr, addr common.Address) (common.Address, error) {
	var is common.Address

	client := getClient(endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(rAddr, client)
	if err != nil {
		return is, err
	}

	retryCount := 0
	for {
		retryCount++
		is, err = roleIns.Issuance(&bind.CallOpts{
			From: addr,
		})
		if err != nil {
			if retryCount > sendTransactionRetryCount {
				return is, err
			}
			time.Sleep(retryGetInfoSleepTime)
			continue
		}

		return is, nil
	}
}
