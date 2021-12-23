package settle

import (
	"context"
	"crypto/ecdsa"
	"encoding/binary"
	"encoding/hex"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gogo/protobuf/proto"
	"golang.org/x/xerrors"

	callconts "memoContract/callcontracts"
	iface "memoContract/interfaces"

	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/lib/utils"
)

var _ ISettle = &ContractMgr{}

type ContractMgr struct {
	ctx context.Context

	hexSK   string
	roleID  uint64
	groupID uint64
	level   int
	tIndex  uint32

	eAddr  common.Address // local address
	txOpts *callconts.TxOpts

	rAddr   common.Address // role contract address
	rtAddr  common.Address // token mgr address
	tAddr   common.Address // token address
	ppAddr  common.Address // pledge pool address
	isAddr  common.Address // issurance address
	rfsAddr common.Address // issurance address
	fsAddr  common.Address // fs contract addr

	// Role caller for user
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
	callconts.EndPoint = "http://119.147.213.220:8191"

	txopts := &callconts.TxOpts{
		Nonce:    nil,
		GasPrice: big.NewInt(callconts.DefaultGasPrice),
		GasLimit: callconts.DefaultGasLimit,
	}

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
	val := QueryBalance(eAddr)
	if val.Cmp(big.NewInt(100_000_000_000_000_000)) < 0 {
		TransferTo(eAddr, big.NewInt(100_000_000_000_000_000))
	}

	val = QueryBalance(eAddr)
	logger.Debug("%s has val %d", eAddr, val)
	if val.Cmp(big.NewInt(100_000_000_000_000_000)) < 0 {
		return nil, xerrors.Errorf("val %d is not enough", val)
	}

	rAddr := callconts.RoleAddr
	iRole := callconts.NewR(rAddr, eAddr, hexSk, txopts)

	rtAddr, err := iRole.RToken()
	if err != nil {
		return nil, err
	}

	iRT := callconts.NewRT(rtAddr, eAddr, hexSk, txopts)
	tIndex := uint32(0)
	tAddr, err := iRT.GetTA(tIndex)
	if err != nil {
		return nil, err
	}
	iERC := callconts.NewERC20(tAddr, eAddr, hexSk, txopts)

	// transfer erc20
	val, err = iERC.BalanceOf(eAddr)
	if err != nil {
		return nil, err
	}
	if val.Cmp(big.NewInt(100_000_000_000_000_000)) < 0 {
		erc20Transfer(eAddr, big.NewInt(100_000_000_000_000_000))
	}
	val, err = iERC.BalanceOf(eAddr)
	if err != nil {
		return nil, err
	}
	logger.Debug("%s has erc20 val %d", eAddr, val)

	ppAddr, err := iRole.PledgePool()
	if err != nil {
		return nil, err
	}
	ipp := callconts.NewPledgePool(ppAddr, eAddr, hexSk, txopts)

	rfsAddr, err := iRole.Rolefs()
	if err != nil {
		return nil, err
	}
	iRFS := callconts.NewRFS(rfsAddr, eAddr, hexSk, txopts)

	isAddr, err := iRole.Issuance()
	if err != nil {
		return nil, err
	}
	iIS := callconts.NewIssu(isAddr, eAddr, hexSk, txopts)

	cm := &ContractMgr{
		ctx: ctx,

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

	return cm, nil
}

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

	if rid == 0 {
		// registerRole
		err := cm.RegisterRole()
		if err != nil {
			return err
		}
	}

	_, _, rType, rid, gid, _, err := cm.iRole.GetRoleInfo(cm.eAddr)
	if err != nil {
		return err
	}

	logger.Debug("get roleinfo: ", rid, gid, rType)

	if rid == 0 {
		return xerrors.Errorf("register fails")
	}

	cm.roleID = rid

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

	_, _, rType, rid, gid, _, err = cm.iRole.GetRoleInfo(cm.eAddr)
	if err != nil {
		return err
	}

	logger.Debug("get roleinfo: ", rid, gid, rType)

	switch typ {
	case pb.RoleInfo_Keeper:
		if rType != callconts.KeeperRoleType {
			return xerrors.Errorf("role type wrong, expected %d, got %d", callconts.KeeperRoleType, typ)
		}

		if gid == 0 && gIndex > 0 {
			err = cm.AddKeeperToGroup(gIndex)
			if err != nil {
				return err
			}

			_, _, rType, _, gid, _, err = cm.iRole.GetRoleInfo(cm.eAddr)
			if err != nil {
				return err
			}

			if gid != gIndex {
				return xerrors.Errorf("group is wrong, expected %d, got %d", gIndex, gid)
			}
		}
	case pb.RoleInfo_Provider:
		if rType != callconts.ProviderRoleType {
			return xerrors.Errorf("role type wrong, expected %d, got %d", callconts.ProviderRoleType, typ)
		}

		if gid == 0 && gIndex > 0 {
			err = cm.AddProviderToGroup(gIndex)
			if err != nil {
				return err
			}

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
			return xerrors.Errorf("role type wrong, expected %d, got %d", callconts.UserRoleType, typ)
		}

		_, _, rType, _, gid, _, err = cm.iRole.GetRoleInfo(cm.eAddr)
		if err != nil {
			return err
		}

		if gIndex > 0 && gid != gIndex {
			return xerrors.Errorf("group is wrong, expected %d, got %d", gIndex, gid)
		}
	default:
	}

	cm.roleID = rid
	cm.groupID = gIndex

	if gIndex > 0 {
		fsAddr, level, err := cm.getGroupInfo(gIndex)
		if err != nil {
			return err
		}
		cm.fsAddr = fsAddr
		cm.level = level
		cm.iFS = callconts.NewFileSys(fsAddr, cm.eAddr, cm.hexSK, cm.txOpts)
	}

	if rType == 1 {
		cm.Recharge()
	}

	return nil
}

// get info
func (cm *ContractMgr) GetRoleInfo(addr address.Address) (*pb.RoleInfo, error) {
	eAddr := common.BytesToAddress(utils.ToEthAddress(addr.Bytes()))
	return cm.getRoleInfo(eAddr)
}

func (cm *ContractMgr) GetRoleInfoAt(rid uint64) (*pb.RoleInfo, error) {
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

func (cm *ContractMgr) RegisterRole() error {
	logger.Debug("contract mgr register role")
	return cm.iRole.Register(cm.eAddr, nil)
}

func (cm *ContractMgr) getGroupInfo(gIndex uint64) (common.Address, int, error) {
	isActive, isBanned, isReady, level, _, _, fsAddr, err := cm.iRole.GetGroupInfo(gIndex)
	if err != nil {
		return common.Address{}, 0, err
	}

	logger.Debugf("group %d, state %v %v %v, level %d, fsAddr %s", gIndex, isActive, isBanned, isReady, level, fsAddr)

	return fsAddr, int(level), nil
}

func (cm *ContractMgr) GetRoleID() uint64 {
	return cm.roleID
}

func (cm *ContractMgr) GetGroupID() uint64 {
	return cm.groupID
}

func (cm *ContractMgr) GetThreshold() int {
	return cm.level
}

func (cm *ContractMgr) GetAllAddrs(ds store.KVStore) {
	//get all addrs and added it into roleMgr
	tc := time.NewTicker(30 * time.Second)
	defer tc.Stop()

	key := store.NewKey(pb.MetaType_RoleInfoKey)

	kCnt := uint64(0)
	pCnt := uint64(0)
	uCnt := uint64(0)

	val, _ := ds.Get(key)
	if len(val) >= 24 {
		kCnt = binary.BigEndian.Uint64(val[:8])
		pCnt = binary.BigEndian.Uint64(val[8:16])
		uCnt = binary.BigEndian.Uint64(val[16:24])
	}

	for {
		select {
		case <-cm.ctx.Done():
			return
		case <-tc.C:
			if cm.groupID == 0 {
				continue
			}

			kcnt, err := cm.iRole.GetGKNum(cm.groupID)
			if err != nil {
				logger.Debugf("get group %d keeper count fail %w", cm.groupID, err)
				continue
			}
			if kcnt > kCnt {
				nkey := store.NewKey(pb.MetaType_RoleInfoKey, cm.groupID, pb.RoleInfo_Keeper.String())
				data, _ := ds.Get(nkey)
				for i := kCnt; i < kcnt; i++ {
					kindex, err := cm.iRole.GetGroupK(cm.groupID, i)
					if err != nil {
						logger.Debugf("get group %d keeper %d fail %w", cm.groupID, i, err)
						continue
					}

					buf := make([]byte, 8)
					binary.BigEndian.PutUint64(buf, kindex)
					data = append(data, buf...)

					pri, err := cm.GetRoleInfoAt(kindex)
					if err != nil {
						continue
					}

					val, err := proto.Marshal(pri)
					if err != nil {
						continue
					}

					// save to local
					nrkey := store.NewKey(pb.MetaType_RoleInfoKey, pri.ID)
					ds.Put(nrkey, val)
				}
				kCnt = kcnt
				ds.Put(nkey, data)
			}

			ucnt, pcnt, err := cm.iRole.GetGUPNum(cm.groupID)
			if err != nil {
				logger.Debugf("get group %d user-pro count fail %w", cm.groupID, err)
				continue
			}
			if pcnt > pCnt {
				nkey := store.NewKey(pb.MetaType_RoleInfoKey, cm.groupID, pb.RoleInfo_Provider.String())
				data, _ := ds.Get(nkey)

				for i := pCnt; i < pcnt; i++ {
					pindex, err := cm.iRole.GetGroupP(cm.groupID, i)
					if err != nil {
						logger.Debugf("get group %d pro %d fail %w", cm.groupID, i, err)
						continue
					}

					buf := make([]byte, 8)
					binary.BigEndian.PutUint64(buf, pindex)
					data = append(data, buf...)

					pri, err := cm.GetRoleInfoAt(pindex)
					if err != nil {
						continue
					}

					val, err := proto.Marshal(pri)
					if err != nil {
						continue
					}

					// save to local
					nrkey := store.NewKey(pb.MetaType_RoleInfoKey, pri.ID)
					ds.Put(nrkey, val)
				}
				pCnt = pcnt

				kCnt = kcnt
				ds.Put(nkey, data)
			}

			if ucnt > uCnt {
				for i := uCnt; i < ucnt; i++ {
					uindex, err := cm.iRole.GetGroupU(cm.groupID, i)
					if err != nil {
						logger.Debugf("get group %d user %d fail %w", cm.groupID, i, err)
						continue
					}

					pri, err := cm.GetRoleInfoAt(uindex)
					if err != nil {
						continue
					}

					val, err := proto.Marshal(pri)
					if err != nil {
						continue
					}

					// save to local
					nkey := store.NewKey(pb.MetaType_RoleInfoKey, pri.ID)
					ds.Put(nkey, val)
				}
				uCnt = ucnt
			}

			logger.Debug("sync from chain: ", kCnt, pCnt, uCnt)

			buf := make([]byte, 24)
			binary.BigEndian.PutUint64(buf[:8], kCnt)
			binary.BigEndian.PutUint64(buf[8:16], pCnt)
			binary.BigEndian.PutUint64(buf[16:24], uCnt)
			ds.Put(key, buf)
		}
	}
}
