package settle

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"golang.org/x/xerrors"

	callconts "memoc/callcontracts"
	"memoc/contracts/role"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/utils"
)

var _ api.ISettle = &ContractMgr{}

type ContractMgr struct {
	ctx context.Context

	endPoint string

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
	logger.Debug("create contract mgr: ", endPoint, roleAddr)

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
	tIndex := uint32(0)
	cm := &ContractMgr{
		ctx:      ctx,
		endPoint: endPoint,

		hexSK:  hexSk,
		tIndex: tIndex,

		eAddr: eAddr,
		rAddr: rAddr,
	}

	rtAddr, err := cm.getRoleTokenAddr()
	if err != nil {
		return nil, err
	}
	cm.rtAddr = rtAddr

	tAddr, err := cm.getTokenAddr(cm.tIndex)
	if err != nil {
		return nil, err
	}
	cm.tAddr = tAddr

	val = cm.getBalanceInErc(eAddr)
	logger.Debugf("%s has erc20 val %d", eAddr, val)

	//if val.BitLen() == 0 {
	//	return nil, xerrors.Errorf("not have erc20 token")
	//}

	ppAddr, err := cm.getPledgeAddr()
	if err != nil {
		return nil, err
	}
	cm.ppAddr = ppAddr

	rfsAddr, err := cm.getRolefsAddr()
	if err != nil {
		return nil, err
	}
	cm.rfsAddr = rfsAddr

	isAddr, err := cm.getIssuanceAddr()
	if err != nil {
		return nil, err
	}
	cm.isAddr = isAddr

	logger.Debug("role contract address: ", rAddr.Hex())
	logger.Debug("token mgr contract address: ", rtAddr.Hex())
	logger.Debug("token contract address: ", tAddr.Hex())
	logger.Debug("role fs contract address: ", rfsAddr.Hex())
	logger.Debug("issu contract address: ", isAddr.Hex())
	logger.Debug("pledge contract address: ", ppAddr.Hex())

	return cm, nil
}

func (cm *ContractMgr) getRoleTokenAddr() (common.Address, error) {
	var rt common.Address

	client := getClient(cm.endPoint)
	defer client.Close()

	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return rt, err
	}

	retryCount := 0
	for {
		retryCount++
		rt, err = roleIns.RToken(&bind.CallOpts{
			From: cm.eAddr,
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

func (cm *ContractMgr) getTokenAddr(tIndex uint32) (common.Address, error) {
	var taddr common.Address

	client := getClient(cm.endPoint)
	defer client.Close()
	rToken, err := role.NewRToken(cm.rAddr, client)
	if err != nil {
		return taddr, err
	}

	retryCount := 0
	for {
		retryCount++
		taddr, err = rToken.GetTA(&bind.CallOpts{
			From: cm.eAddr,
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

func (cm *ContractMgr) getPledgeAddr() (common.Address, error) {
	var pp common.Address
	client := getClient(cm.endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return pp, err
	}

	retryCount := 0
	for {
		retryCount++
		pp, err = roleIns.PledgePool(&bind.CallOpts{
			From: cm.eAddr,
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

func (cm *ContractMgr) getRolefsAddr() (common.Address, error) {
	var rfs common.Address

	client := getClient(cm.endPoint)
	defer client.Close()

	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return rfs, err
	}

	retryCount := 0
	for {
		retryCount++
		rfs, err = roleIns.Rolefs(&bind.CallOpts{
			From: cm.eAddr,
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

func (cm *ContractMgr) getIssuanceAddr() (common.Address, error) {
	var is common.Address

	client := getClient(cm.endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return is, err
	}

	retryCount := 0
	for {
		retryCount++
		is, err = roleIns.Issuance(&bind.CallOpts{
			From: cm.eAddr,
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

// register account and register role
func Register(ctx context.Context, endPoint, rAddr string, sk []byte, typ pb.RoleInfo_Type, gIndex uint64) (uint64, uint64, error) {
	cm, err := NewContractMgr(ctx, endPoint, rAddr, sk)
	if err != nil {
		return 0, 0, err
	}

	err = cm.Start(typ, gIndex)
	if err != nil {
		return 0, 0, err
	}

	return cm.roleID, cm.groupID, nil
}

func (cm *ContractMgr) Start(typ pb.RoleInfo_Type, gIndex uint64) error {
	if gIndex == 0 {
		return nil
	}
	logger.Debug("start contract mgr: ", typ, gIndex)
	ri, err := cm.getRoleInfo(cm.eAddr)
	if err != nil {
		return err
	}

	logger.Debug("get roleinfo: ", ri.pri.ID)

	// register account
	if ri.pri.ID == 0 {
		err := cm.RegisterAcc()
		if err != nil {
			return err
		}
	}

	ri, err = cm.getRoleInfo(cm.eAddr)
	if err != nil {
		return err
	}

	logger.Debug("get roleinfo after register account: ", ri)

	if ri.pri.ID == 0 {
		return xerrors.Errorf("register fails")
	}

	cm.roleID = ri.pri.ID

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

	logger.Debug("get roleinfo after register role: ", ri)

	switch typ {
	case pb.RoleInfo_Keeper:
		if ri.pri.Type != typ {
			return xerrors.Errorf("role type wrong, expected %s, got %s", pb.RoleInfo_Keeper, ri.pri.Type)
		}

		if ri.pri.GroupID == 0 && gIndex > 0 {
			fmt.Println("need grant to add keeper to group")
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
				return xerrors.Errorf("group is wrong, expected %d, got %d", gIndex, ri.pri.GroupID)
			}
		}
	case pb.RoleInfo_User:
		if ri.pri.Type != typ {
			return xerrors.Errorf("role type wrong, expected %s, got %s", pb.RoleInfo_User, ri.pri.Type)
		}
	default:
	}

	cm.roleID = ri.pri.ID

	if ri.pri.GroupID == 0 {
		cm.groupID = gIndex
	} else {
		cm.groupID = ri.pri.GroupID
	}

	if gIndex > 0 {
		gi, err := cm.SettleGetGroupInfoAt(cm.ctx, gIndex)
		if err != nil {
			return err
		}
		cm.fsAddr = common.HexToAddress(gi.FsAddr)
		cm.level = int(gi.Level)
		logger.Debug("fs contract address: ", cm.fsAddr.Hex())
	}

	return nil
}

// as net prefix
func (cm *ContractMgr) GetRoleAddr() common.Address {
	return cm.rAddr
}

// get info
func (cm *ContractMgr) SettleGetRoleInfo(addr address.Address) (*pb.RoleInfo, error) {
	eAddr := common.BytesToAddress(utils.ToEthAddress(addr.Bytes()))
	ri, err := cm.getRoleInfo(eAddr)
	if err != nil {
		return nil, err
	}

	return ri.pri, nil
}

func (cm *ContractMgr) SettleGetRoleInfoAt(ctx context.Context, rid uint64) (*pb.RoleInfo, error) {
	gotAddr, err := cm.getAddrAt(rid)
	if err != nil {
		return nil, err
	}

	ri, err := cm.getRoleInfo(gotAddr)
	if err != nil {
		return nil, err
	}
	return ri.pri, nil
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
		ri.pri.ID = rid
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

func (cm *ContractMgr) RegisterAcc() error {
	logger.Debug("contract mgr register an account to get an accIndex for it.")

	client := getClient(cm.endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return err
	}

	// check if addr has registered
	ri, err := cm.getRoleInfo(cm.eAddr)
	if ri.pri.ID > 0 { // has registered already
		return nil
	}

	logger.Debug("begin Register in Role contract...")

	// txopts.gasPrice参数赋值为nil
	auth, errMA := makeAuth(cm.hexSK, nil, nil)
	if errMA != nil {
		return errMA
	}
	tx, err := roleIns.Register(auth, cm.eAddr, nil)
	if err != nil {
		return err
	}
	return checkTx(cm.endPoint, tx, "Register")
}

func (cm *ContractMgr) getGroupInfo(gIndex uint64) (bool, bool, bool, uint16, *big.Int, *big.Int, common.Address, error) {
	retryCount := 0

	var isActive, isBanned, isReady bool
	var level uint16
	var size, price *big.Int
	var fsAddr common.Address

	if gIndex == 0 {
		return isActive, isBanned, isReady, level, size, price, fsAddr, xerrors.Errorf("group is zero")
	}

	client := getClient(cm.endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return isActive, isBanned, isReady, level, size, price, fsAddr, err
	}

	gIndex-- // get group info by array index actually in contract, which is gIndex minus 1
	for {
		retryCount++
		isActive, isBanned, isReady, level, size, price, fsAddr, err = roleIns.GetGroupInfo(&bind.CallOpts{
			From: cm.eAddr,
		}, gIndex)
		if err != nil {
			if retryCount > sendTransactionRetryCount {
				return isActive, isBanned, isReady, level, size, price, fsAddr, err
			}
			time.Sleep(retryGetInfoSleepTime)
			continue
		}

		return isActive, isBanned, isReady, level, size, price, fsAddr, nil
	}
}

// GetGKNum get the number of keepers in the group.
func (cm *ContractMgr) getKNumAtGroup(gIndex uint64) (uint64, error) {
	var gkNum uint64

	if gIndex == 0 {
		return gkNum, xerrors.Errorf("group index is zero")
	}

	client := getClient(cm.endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return gkNum, err
	}

	retryCount := 0

	gIndex--
	for {
		retryCount++
		gkNum, err = roleIns.GetGKNum(&bind.CallOpts{
			From: cm.eAddr,
		}, gIndex)
		if err != nil {
			if retryCount > sendTransactionRetryCount {
				return gkNum, err
			}
			time.Sleep(retryGetInfoSleepTime)
			continue
		}

		return gkNum, nil
	}
}

// GetGUPNum get the number of user、providers in the group.
func (cm *ContractMgr) getUPNumAtGroup(gIndex uint64) (uint64, uint64, error) {
	var gpNum, guNum uint64

	if gIndex == 0 {
		return gpNum, guNum, xerrors.Errorf("group index is zero")
	}

	client := getClient(cm.endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return gpNum, guNum, err
	}

	retryCount := 0
	gIndex--
	for {
		retryCount++
		guNum, gpNum, err = roleIns.GetGUPNum(&bind.CallOpts{
			From: cm.eAddr,
		}, gIndex)
		if err != nil {
			if retryCount > sendTransactionRetryCount {
				return guNum, gpNum, err
			}
			time.Sleep(retryGetInfoSleepTime)
			continue
		}

		return guNum, gpNum, nil
	}
}

func (cm *ContractMgr) SettleGetGroupInfoAt(ctx context.Context, gIndex uint64) (*api.GroupInfo, error) {
	isActive, isBanned, isReady, level, size, price, fsAddr, err := cm.getGroupInfo(gIndex)
	if err != nil {
		return nil, err
	}

	logger.Debugf("group %d, state %v %v %v, level %d, fsAddr %s", gIndex, isActive, isBanned, isReady, level, fsAddr)

	kc, err := cm.getKNumAtGroup(cm.groupID)
	if err != nil {
		return nil, err
	}

	uc, pc, err := cm.getUPNumAtGroup(cm.groupID)
	if err != nil {
		return nil, err
	}

	gi := &api.GroupInfo{
		RoleAddr: cm.rAddr.String(),
		ID:       gIndex,
		Level:    level,
		FsAddr:   fsAddr.String(),
		Size:     size.Uint64(),
		Price:    new(big.Int).Set(price),
		KCount:   kc,
		UCount:   uc,
		PCount:   pc,
	}

	return gi, nil
}

func (cm *ContractMgr) SettleGetRoleID(ctx context.Context) uint64 {
	return cm.roleID
}

func (cm *ContractMgr) SettleGetGroupID(ctx context.Context) uint64 {
	return cm.groupID
}

func (cm *ContractMgr) SettleGetThreshold(ctx context.Context) int {
	return cm.level
}

func (cm *ContractMgr) GetGKNum() (uint64, error) {
	return cm.getKNumAtGroup(cm.groupID)
}

func (cm *ContractMgr) GetGUPNum() (uint64, uint64, error) {
	return cm.getUPNumAtGroup(cm.groupID)
}

// GetGroupK get keeper role index by gIndex and keeper array index.
func (cm *ContractMgr) GetGroupK(gIndex uint64, index uint64) (uint64, error) {
	var kIndex uint64

	if gIndex == 0 {
		return kIndex, xerrors.Errorf("group index is zero")
	}

	gkNum, err := cm.getKNumAtGroup(gIndex)
	if err != nil {
		return kIndex, err
	}
	if index >= gkNum {
		return kIndex, xerrors.Errorf("index %s is larger than group keeper count %d", index, gkNum)
	}

	client := getClient(cm.endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return kIndex, err
	}

	retryCount := 0
	gIndex-- // get group info by array index actually in contract, which is gIndex minus 1
	for {
		retryCount++
		kIndex, err = roleIns.GetGroupK(&bind.CallOpts{
			From: cm.eAddr,
		}, gIndex, index)
		if err != nil {
			if retryCount > sendTransactionRetryCount {
				return kIndex, err
			}
			time.Sleep(retryGetInfoSleepTime)
			continue
		}

		return kIndex, nil
	}
}

// GetGroupP get provider role index by gIndex and provider array index.
func (cm *ContractMgr) GetGroupP(gIndex uint64, index uint64) (uint64, error) {
	var pIndex uint64

	if gIndex == 0 {
		return pIndex, xerrors.Errorf("group index is zero")
	}

	pkNum, _, err := cm.getUPNumAtGroup(gIndex)
	if err != nil {
		return pIndex, err
	}
	if index >= pkNum {
		return pIndex, xerrors.Errorf("index %s is larger than group provider count %d", index, pkNum)
	}

	client := getClient(cm.endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return pIndex, err
	}

	retryCount := 0
	gIndex-- // get group info by array index actually in contract, which is gIndex minus 1
	for {
		retryCount++
		pIndex, err = roleIns.GetGroupP(&bind.CallOpts{
			From: cm.eAddr,
		}, gIndex, index)
		if err != nil {
			if retryCount > sendTransactionRetryCount {
				return pIndex, err
			}
			time.Sleep(retryGetInfoSleepTime)
			continue
		}

		return pIndex, nil
	}
}

// GetGroupU get user role index by gIndex and user array index.
func (cm *ContractMgr) GetGroupU(gIndex uint64, index uint64) (uint64, error) {
	var uIndex uint64

	if gIndex == 0 {
		return uIndex, xerrors.Errorf("group index is zero")
	}

	_, ukNum, err := cm.getUPNumAtGroup(gIndex)
	if err != nil {
		return uIndex, err
	}
	if index >= ukNum {
		return uIndex, xerrors.Errorf("index %s is larger than group user count %d", index, ukNum)
	}

	client := getClient(cm.endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return uIndex, err
	}

	retryCount := 0

	gIndex-- // get group info by array index actually in contract, which is gIndex minus 1
	for {
		retryCount++
		uIndex, err = roleIns.GetGU(&bind.CallOpts{
			From: cm.eAddr,
		}, gIndex, index)
		if err != nil {
			if retryCount > sendTransactionRetryCount {
				return uIndex, err
			}
			time.Sleep(retryGetInfoSleepTime)
			continue
		}

		return uIndex, nil
	}
}
