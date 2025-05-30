package settle

import (
	"context"
	"encoding/hex"
	"math/big"

	"github.com/zeebo/blake3"
	"golang.org/x/xerrors"

	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12_381"
	"github.com/memoio/go-mefs-v2/lib/crypto/pdp"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
)

// register account and register role
func Register(ctx context.Context, endPoint, rAddr string, ver uint32, sk []byte, typ pb.RoleInfo_Type, gIndex uint64) (uint64, uint64, error) {
	cm, err := NewContractMgr(ctx, endPoint, rAddr, ver, sk)
	if err != nil {
		return 0, 0, err
	}

	err = cm.Start(typ, gIndex)
	if err != nil {
		return 0, 0, err
	}

	return cm.SettleGetRoleID(ctx), cm.SettleGetGroupID(ctx), nil
}

func (cm *ContractMgr) RegisterAccount() error {
	logger.Info("register account to get an unique ID")

	err := cm.proxyIns.ReAcc()
	if err != nil {
		return err
	}
	return nil
}

// Register role
func (cm *ContractMgr) RegisterRole(typ pb.RoleInfo_Type) error {
	var rtype uint8
	var extra []byte

	switch typ {
	case pb.RoleInfo_Keeper:
		rtype = 3
		skByte, err := hex.DecodeString(cm.sk)
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
		extra = blskey
	case pb.RoleInfo_Provider:
		rtype = 2
	case pb.RoleInfo_User:
		rtype = 1
		skByte, err := hex.DecodeString(cm.sk)
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
		extra = pdpKeySet.VerifyKey().Serialize()
	default:
		return xerrors.Errorf("Register role unsupported role type %s", typ)
	}

	logger.Info("register role: ", typ)
	err := cm.proxyIns.ReRole(rtype, extra)
	if err != nil {
		return err
	}
	return nil
}

func (cm *ContractMgr) AddToGroup(gi uint64) error {
	pri, err := cm.getIns.GetPleRewardInfo(cm.roleID, cm.tIndex)
	if err != nil {
		return err
	}

	ginfo, err := cm.getIns.GetGroupInfo(gi)
	if err != nil {
		return err
	}

	require := new(big.Int)

	switch cm.typ {
	case pb.RoleInfo_Keeper:
		require.Set(ginfo.Kpr)
	case pb.RoleInfo_Provider:
		require.Set(ginfo.Ppr)
	case pb.RoleInfo_User:
	default:
		return xerrors.Errorf("unsupported role type %s", cm.typ)
	}

	// check pledge is enough
	if require.Cmp(pri.Last) > 0 {
		require.Sub(require, pri.Last)

		err := cm.Pledge(cm.roleID, require)
		if err != nil {
			return err
		}
	}

	logger.Infof("add to group %d", gi)
	err = cm.proxyIns.AddToGroup(gi)
	if err != nil {
		return err
	}

	return nil
}

func (cm *ContractMgr) Pledge(roleID uint64, val *big.Int) error {
	// check erc20
	bal := cm.ercIns.BalanceOf(cm.eAddr)
	if val.Cmp(bal) > 0 {
		return xerrors.Errorf("pledge balance not enough, need %d, has %d", val, bal)
	}

	ppool, err := cm.getIns.GetPledgePool()
	if err != nil {
		return err
	}

	// check allowance
	al := cm.ercIns.Allowance(cm.eAddr, ppool)
	if val.Cmp(al) > 0 {
		logger.Debugf("Approve %d in pool %s", val, ppool)
		err := cm.ercIns.Approve(ppool, val)
		if err != nil {
			return err
		}
	}

	logger.Debugf("Pledge %d", val)
	err = cm.proxyIns.Pledge(roleID, val)
	if err != nil {
		return err
	}

	return nil
}
