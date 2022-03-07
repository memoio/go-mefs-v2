package settle

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"golang.org/x/xerrors"

	callconts "memoc/callcontracts"
	iface "memoc/interfaces"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/utils"
)

var _ api.ISettle = &ContractMgr{}

type ContractMgr struct {
	ctx context.Context

	hexSK   string
	roleID  uint64
	groupID uint64
	level   int
	tIndex  uint32

	status chan error

	eAddr  common.Address // local address
	txOpts *callconts.TxOpts

	rAddr   common.Address // role contract address
	rtAddr  common.Address // token mgr address
	tAddr   common.Address // token address
	ppAddr  common.Address // pledge pool address
	isAddr  common.Address // issurance address
	rfsAddr common.Address // issurance address
	fsAddr  common.Address // fs contract addr

	// caller
	iRole iface.RoleInfo
	iRT   iface.RTokenInfo
	iErc  iface.ERC20Info
	iPP   iface.PledgePoolInfo
	iSS   iface.IssuanceInfo
	iRFS  iface.RoleFSInfo
	iFS   iface.FileSysInfo
}

func NewContractMgr(ctx context.Context, sk []byte) (*ContractMgr, error) {
	logger.Debug("create contract mgr")
	// set endpoint

	txopts := &callconts.TxOpts{
		Nonce:    nil,
		GasPrice: big.NewInt(callconts.DefaultGasPrice),
		GasLimit: callconts.DefaultGasLimit,
	}

	status := make(chan error)

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
	fmt.Println("caller addr: ", eAddr)

	// transfer eth
	// todo: remove at mainnet
	val := QueryBalance(eAddr)
	logger.Debugf("%s has val %d", eAddr, val)
	if val.BitLen() == 0 {
		return nil, xerrors.Errorf("not have tx fee on chain")
	}

	rAddr := RoleAddr
	iRole := callconts.NewR(rAddr, eAddr, hexSk, txopts, endpoint, status)

	rtAddr, err := iRole.RToken()
	if err != nil {
		return nil, err
	}

	iRT := callconts.NewRT(rtAddr, eAddr, hexSk, txopts, endpoint)
	tIndex := uint32(0)
	tAddr, err := iRT.GetTA(tIndex)
	if err != nil {
		return nil, err
	}
	iERC := callconts.NewERC20(tAddr, eAddr, hexSk, txopts, endpoint, status)

	// transfer erc20
	val, err = iERC.BalanceOf(eAddr)
	if err != nil {
		return nil, err
	}
	logger.Debugf("%s has erc20 val %d", eAddr, val)

	//if val.BitLen() == 0 {
	//	return nil, xerrors.Errorf("not have erc20 token")
	//}

	ppAddr, err := iRole.PledgePool()
	if err != nil {
		return nil, err
	}
	ipp := callconts.NewPledgePool(ppAddr, eAddr, hexSk, txopts, endpoint, status)

	rfsAddr, err := iRole.Rolefs()
	if err != nil {
		return nil, err
	}
	iRFS := callconts.NewRFS(rfsAddr, eAddr, hexSk, txopts, endpoint, status)

	isAddr, err := iRole.Issuance()
	if err != nil {
		return nil, err
	}
	iIS := callconts.NewIssu(isAddr, eAddr, hexSk, txopts, endpoint, status)

	cm := &ContractMgr{
		ctx:    ctx,
		status: status,

		hexSK:  hexSk,
		tIndex: tIndex,

		eAddr:  eAddr,
		txOpts: txopts,

		rAddr:   rAddr,
		rtAddr:  rtAddr,
		tAddr:   tAddr,
		rfsAddr: rfsAddr,
		isAddr:  isAddr,
		ppAddr:  ppAddr,

		iRole: iRole,
		iRT:   iRT,
		iErc:  iERC,
		iRFS:  iRFS,
		iSS:   iIS,
		iPP:   ipp,
	}

	logger.Debug("role contract address: ", rAddr.Hex())
	logger.Debug("token mgr contract address: ", rtAddr.Hex())
	logger.Debug("token contract address: ", tAddr.Hex())
	logger.Debug("role fs contract address: ", rfsAddr.Hex())
	logger.Debug("issu contract address: ", isAddr.Hex())
	logger.Debug("pledge contract address: ", ppAddr.Hex())

	return cm, nil
}

// register account and register role
func Register(ctx context.Context, sk []byte, typ pb.RoleInfo_Type, gIndex uint64) (uint64, uint64, error) {
	cm, err := NewContractMgr(ctx, sk)
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
	_, _, _, rid, _, _, err := cm.iRole.GetRoleInfo(cm.eAddr)
	if err != nil {
		return err
	}

	logger.Debug("get roleinfo: ", rid)

	// register account
	if rid == 0 {
		err := cm.RegisterAcc()
		if err != nil {
			return err
		}
	}

	_, _, rType, rid, gid, _, err := cm.iRole.GetRoleInfo(cm.eAddr)
	if err != nil {
		return err
	}

	logger.Debug("get roleinfo after register account: ", rid, gid, rType)

	if rid == 0 {
		return xerrors.Errorf("register fails")
	}

	cm.roleID = rid

	// register role
	if rType == 0 {
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

	_, _, rType, rid, gid, _, err = cm.iRole.GetRoleInfo(cm.eAddr)
	if err != nil {
		return err
	}

	logger.Debug("get roleinfo after register role: ", rid, gid, rType)

	switch typ {
	case pb.RoleInfo_Keeper:
		if rType != callconts.KeeperRoleType {
			return xerrors.Errorf("role type wrong, expected %d, got %d", callconts.KeeperRoleType, rType)
		}

		if gid == 0 && gIndex > 0 {
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
		if rType != callconts.ProviderRoleType {
			return xerrors.Errorf("role type wrong, expected %d, got %d", callconts.ProviderRoleType, rType)
		}

		if gid == 0 && gIndex > 0 {
			err = cm.AddProviderToGroup(gIndex)
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
		}
	case pb.RoleInfo_User:
		if rType != callconts.UserRoleType {
			return xerrors.Errorf("role type wrong, expected %d, got %d", callconts.UserRoleType, rType)
		}
	default:
	}

	cm.roleID = rid

	if gid == 0 {
		cm.groupID = gIndex
	} else {
		cm.groupID = gid
	}

	if gIndex > 0 {
		gi, err := cm.SettleGetGroupInfoAt(cm.ctx, gIndex)
		if err != nil {
			return err
		}
		cm.fsAddr = common.HexToAddress(gi.FsAddr)
		cm.level = int(gi.Level)
		cm.iFS = callconts.NewFileSys(cm.fsAddr, cm.eAddr, cm.hexSK, cm.txOpts, endpoint, cm.status)
		logger.Debug("fs contract address: ", cm.fsAddr.Hex())
	}

	//if rType == 1 {
	//	cm.Recharge(big.NewInt(10_000_000_000_000_000))
	//}

	return nil
}

// get info
func (cm *ContractMgr) SettleGetRoleInfo(addr address.Address) (*pb.RoleInfo, error) {
	eAddr := common.BytesToAddress(utils.ToEthAddress(addr.Bytes()))
	return cm.getRoleInfo(eAddr)
}

func (cm *ContractMgr) SettleGetRoleInfoAt(ctx context.Context, rid uint64) (*pb.RoleInfo, error) {
	gotAddr, err := cm.iRole.GetAddr(rid)
	if err != nil {
		return nil, err
	}

	return cm.getRoleInfo(gotAddr)
}

func (cm *ContractMgr) getRoleInfo(eAddr common.Address) (*pb.RoleInfo, error) {
	_, _, rType, rid, gid, extra, err := cm.iRole.GetRoleInfo(eAddr)
	if err != nil {
		return nil, err
	}

	if rid == 0 {
		return nil, xerrors.Errorf("%s is not register or in group", eAddr)
	}

	//if gid != cm.groupID {
	//	return nil, xerrors.Errorf("%s is not register in group %d, got %d", eAddr, cm.groupID, gid)
	//}

	pri := new(pb.RoleInfo)
	pri.ID = rid
	pri.GroupID = gid
	pri.ChainVerifyKey = eAddr.Bytes()

	switch rType {
	case callconts.UserRoleType:
		pri.Type = pb.RoleInfo_User
		pri.Extra = extra
	case callconts.ProviderRoleType:
		pri.Type = pb.RoleInfo_Provider
	case callconts.KeeperRoleType:
		pri.Type = pb.RoleInfo_Keeper
		pri.BlsVerifyKey = extra
	default:
		pri.Type = pb.RoleInfo_Unknown
	}

	return pri, nil
}

func (cm *ContractMgr) RegisterAcc() error {
	logger.Debug("contract mgr register an account to get an accIndex for it.")
	// call role.Register
	err := cm.iRole.Register(cm.eAddr, nil)
	if err != nil {
		return err
	}
	if err = <-cm.status; err != nil {
		logger.Fatal("register role fail: ", err)
	}
	return err
}

func (cm *ContractMgr) SettleGetGroupInfoAt(ctx context.Context, gIndex uint64) (*api.GroupInfo, error) {
	isActive, isBanned, isReady, level, size, price, fsAddr, err := cm.iRole.GetGroupInfo(gIndex)
	if err != nil {
		return nil, err
	}

	logger.Debugf("group %d, state %v %v %v, level %d, fsAddr %s", gIndex, isActive, isBanned, isReady, level, fsAddr)

	kc, err := cm.iRole.GetGKNum(cm.groupID)
	if err != nil {
		return nil, err
	}

	uc, pc, err := cm.iRole.GetGUPNum(cm.groupID)
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
	return cm.iRole.GetGKNum(cm.groupID)
}

func (cm *ContractMgr) GetGUPNum() (uint64, uint64, error) {
	return cm.iRole.GetGUPNum(cm.groupID)
}

func (cm *ContractMgr) GetGroupK(ki uint64) (uint64, error) {
	return cm.iRole.GetGroupK(cm.groupID, ki)
}

func (cm *ContractMgr) GetGroupP(pi uint64) (uint64, error) {
	return cm.iRole.GetGroupP(cm.groupID, pi)
}

func (cm *ContractMgr) GetGroupU(ui uint64) (uint64, error) {
	return cm.iRole.GetGroupP(cm.groupID, ui)
}
