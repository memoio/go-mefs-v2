package settle

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"math/big"

	callconts "memoc/callcontracts"
	"memoc/contracts/role"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/zeebo/blake3"
	"golang.org/x/xerrors"

	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12_381"
	"github.com/memoio/go-mefs-v2/lib/crypto/pdp"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
)

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

func (cm *ContractMgr) RegisterAcc() error {
	logger.Info("Register an account to get an unique ID")

	// check if addr has registered
	ri, err := cm.getRoleInfo(cm.eAddr)
	if err != nil {
		return err
	}

	if ri.pri.RoleID > 0 { // has registered already
		return nil
	}

	logger.Debug("begin Register in Role contract...")

	client := getClient(cm.endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return err
	}

	auth, err := makeAuth(cm.chainID, cm.hexSK, nil, nil)
	if err != nil {
		return err
	}
	tx, err := roleIns.Register(auth, cm.eAddr, nil)
	if err != nil {
		return err
	}
	return checkTx(cm.endPoint, tx, "Register")
}

// RegisterKeeper called by anyone to register Keeper role, befor this, you should pledge in PledgePool
func (cm *ContractMgr) RegisterKeeper() error {
	logger.Info("Register keeper")

	// check index
	addr, err := cm.getAddrAt(cm.roleID)
	if err != nil {
		return err
	}
	ri, err := cm.getRoleInfo(addr)
	if err != nil {
		return err
	}
	if ri.pri.Type != pb.RoleInfo_Unknown {
		return nil
	}

	// check pledge value
	pl, err := cm.getBalanceInPPool(cm.roleID, cm.tIndex)
	if err != nil {
		return err
	}
	pledgek, err := cm.getPledgeK()
	if err != nil {
		return err
	}

	pledgek.Sub(pledgek, pl)
	if pledgek.Cmp(big.NewInt(0)) > 0 {
		err := cm.pledge(pledgek)
		if err != nil {
			return xerrors.Errorf("%d pledge fails %s", cm.roleID, err)
		}
	}

	logger.Debug("begin RegisterKeeper in Role contract...")

	// prepare blskey
	skByte, err := hex.DecodeString(cm.hexSK)
	if err != nil {
		return err
	}
	blsSeed := make([]byte, len(skByte)+1)
	copy(blsSeed[:len(skByte)], skByte)
	blsSeed[len(skByte)] = byte(types.BLS)
	blsByte := blake3.Sum256(blsSeed)
	blskey, err := bls.PublicKey(blsByte[:])
	if err != nil {
		return err
	}

	client := getClient(cm.endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return err
	}

	auth, err := makeAuth(cm.chainID, cm.hexSK, nil, nil)
	if err != nil {
		return err
	}

	tx, err := roleIns.RegisterKeeper(auth, cm.roleID, blskey, nil)
	if err != nil {
		return err
	}

	return checkTx(cm.endPoint, tx, "RegisterKeeper")
}

// RegisterProvider called by anyone to register Provider role, befor this, you should pledge in PledgePool
func (cm *ContractMgr) RegisterProvider() error {
	logger.Info("Register provider")

	pledgep, err := cm.getPledgeP() // 申请Provider最少需质押的金额
	if err != nil {
		return err
	}

	ple, err := cm.getBalanceInPPool(cm.roleID, cm.tIndex)
	if err != nil {
		return err
	}

	pledgep.Sub(pledgep, ple)

	if pledgep.Cmp(big.NewInt(0)) > 0 {
		err = cm.pledge(pledgep)
		if err != nil {
			return err
		}
	}

	// check index
	addr, err := cm.getAddrAt(cm.roleID)
	if err != nil {
		return err
	}
	ri, err := cm.getRoleInfo(addr)
	if err != nil {
		return err
	}
	if ri.pri.Type != pb.RoleInfo_Unknown {
		return nil
	}

	client := getClient(cm.endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return err
	}

	logger.Debug("begin RegisterProvider in Role contract...")

	auth, err := makeAuth(cm.chainID, cm.hexSK, nil, nil)
	if err != nil {
		return err
	}
	tx, err := roleIns.RegisterProvider(auth, cm.roleID, nil)
	if err != nil {
		return err
	}
	return checkTx(cm.endPoint, tx, "RegisterProvider")
}

func (cm *ContractMgr) RegisterUser(gIndex uint64) error {
	logger.Info("Register user")

	// check gindex
	isActive, isBanned, _, _, _, _, _, err := cm.getGroupInfo(gIndex)
	if err != nil {
		return err
	}
	if !isActive {
		return xerrors.Errorf("group %d is not active", gIndex)
	}

	if isBanned {
		return xerrors.Errorf("group %d is banned", gIndex)
	}

	// check index
	addr, err := cm.getAddrAt(cm.roleID)
	if err != nil {
		return err
	}
	ri, err := cm.getRoleInfo(addr)
	if err != nil {
		return err
	}
	if ri.pri.Type != pb.RoleInfo_Unknown {
		return nil
	}

	// prepare bls pubkey
	skByte, err := hex.DecodeString(cm.hexSK)
	if err != nil {
		return err
	}
	blsSeed := make([]byte, len(skByte)+1)
	copy(blsSeed[:len(skByte)], skByte)
	blsSeed[len(skByte)] = byte(types.PDP)

	pdpKeySet, err := pdp.GenerateKeyWithSeed(pdpcommon.PDPV2, blsSeed)
	if err != nil {
		return err
	}

	client := getClient(cm.endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return err
	}

	// don't need to check fs
	logger.Debug("begin RegisterUser in Role contract...")

	auth, err := makeAuth(cm.chainID, cm.hexSK, nil, nil)
	if err != nil {
		return err
	}
	tx, err := roleIns.RegisterUser(auth, cm.roleID, gIndex, pdpKeySet.VerifyKey().Serialize(), nil)
	if err != nil {
		return err
	}

	return checkTx(cm.endPoint, tx, "RegisterUser")
}

func (cm *ContractMgr) AddProviderToGroup(gIndex uint64) error {
	client := getClient(cm.endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return err
	}

	logger.Debug("begin AddProviderToGroup in Role contract...")

	auth, err := makeAuth(cm.chainID, cm.hexSK, nil, nil)
	if err != nil {
		return err
	}
	tx, err := roleIns.AddProviderToGroup(auth, cm.roleID, gIndex, nil)
	if err != nil {
		return err
	}

	return checkTx(cm.endPoint, tx, "AddProviderToGroup")
}

func AddKeeperToGroup(endPoint, roleAddr, sk string, roleID, gIndex uint64) error {
	txopts := &callconts.TxOpts{
		Nonce:    nil,
		GasPrice: big.NewInt(callconts.DefaultGasPrice),
		GasLimit: callconts.DefaultGasLimit,
	}

	privateKey, err := crypto.HexToECDSA(sk)
	if err != nil {
		return err
	}

	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return xerrors.Errorf("error casting public key to ECDSA")
	}

	eAddr := crypto.PubkeyToAddress(*publicKeyECDSA)

	status := make(chan error)
	ar := callconts.NewR(common.HexToAddress(roleAddr), eAddr, sk, txopts, endPoint, status)

	err = ar.AddKeeperToGroup(roleID, gIndex)
	if err != nil {
		return err
	}

	if err = <-status; err != nil {
		logger.Fatal("add keeper to group fail: ", roleID, gIndex, err)
		return err
	}

	return nil
}
