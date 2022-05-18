package role

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/api"
	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12_381"
	"github.com/memoio/go-mefs-v2/lib/crypto/signature"
	"github.com/memoio/go-mefs-v2/lib/crypto/signature/secp256k1"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
)

var logger = logging.Logger("role")

var _ api.IRole = &roleAPI{}

type roleAPI struct {
	*RoleMgr
}

func (rm *RoleMgr) RoleSelf(ctx context.Context) (*pb.RoleInfo, error) {
	rm.Lock()
	defer rm.Unlock()

	return rm.get(rm.roleID)
}

func (rm *RoleMgr) RoleGet(ctx context.Context, id uint64) (*pb.RoleInfo, error) {
	rm.Lock()
	defer rm.Unlock()

	return rm.get(id)
}

func (rm *RoleMgr) RoleGetRelated(ctx context.Context, typ pb.RoleInfo_Type) ([]uint64, error) {
	rm.RLock()
	defer rm.RUnlock()

	switch typ {
	case pb.RoleInfo_Keeper:
		out := make([]uint64, len(rm.keepers))
		for i, id := range rm.keepers {
			out[i] = id
		}

		return out, nil
	case pb.RoleInfo_Provider:
		out := make([]uint64, len(rm.providers))
		for i, id := range rm.providers {
			out[i] = id
		}

		return out, nil
	case pb.RoleInfo_User:
		out := make([]uint64, len(rm.users))
		for i, id := range rm.users {
			out[i] = id
		}
		return out, nil
	default:
		return nil, xerrors.Errorf("roleinfo not supported for type %d", typ)
	}
}

func (rm *RoleMgr) RoleSign(ctx context.Context, id uint64, msg []byte, typ types.SigType) (types.Signature, error) {
	ts := types.Signature{
		Type: typ,
	}

	switch typ {
	case types.SigSecp256k1:
		addr, err := rm.GetPubKey(id, types.Secp256k1)
		if err != nil {
			return ts, err
		}
		sig, err := rm.WalletSign(rm.ctx, addr, msg)
		if err != nil {
			return ts, err
		}
		ts.Data = sig
	case types.SigBLS:
		addr, err := rm.GetPubKey(id, types.BLS)
		if err != nil {
			return ts, err
		}

		sig, err := rm.WalletSign(rm.ctx, addr, msg)
		if err != nil {
			return ts, err
		}
		ts.Data = sig
	default:
		return ts, xerrors.Errorf("sign type %d not supported", typ)
	}

	return ts, nil
}

func (rm *RoleMgr) RoleVerify(ctx context.Context, id uint64, msg []byte, sig types.Signature) (bool, error) {
	var pubByte []byte
	switch sig.Type {
	case types.SigSecp256k1:
		addr, err := rm.GetPubKey(id, types.Secp256k1)
		if err != nil {
			return false, err
		}
		pubByte = addr.Bytes()
	case types.SigBLS:
		addr, err := rm.GetPubKey(id, types.BLS)
		if err != nil {
			return false, err
		}
		pubByte = addr.Bytes()
	default:
		return false, xerrors.Errorf("sign type %d not supported", sig.Type)
	}

	if len(pubByte) == 0 {
		return false, xerrors.Errorf("local has no pubkey for: %d", id)
	}

	ok, err := signature.Verify(pubByte, msg, sig.Data)
	if err != nil {
		return false, err
	}

	return ok, nil
}

func (rm *RoleMgr) RoleVerifyMulti(ctx context.Context, msg []byte, sig types.MultiSignature) (bool, error) {
	err := sig.SanityCheck()
	if err != nil {
		return false, err
	}

	switch sig.Type {
	case types.SigSecp256k1:
		for i, id := range sig.Signer {
			if len(sig.Data) < (i+1)*secp256k1.SignatureSize {
				return false, xerrors.Errorf("sign size wrong")
			}
			addr, err := rm.GetPubKey(id, types.Secp256k1)
			if err != nil {
				return false, err
			}
			sign := sig.Data[i*secp256k1.SignatureSize : (i+1)*secp256k1.SignatureSize]
			ok, err := signature.Verify(addr.Bytes(), msg, sign)
			if err != nil {
				return false, err
			}

			if !ok {
				return false, nil
			}
		}
		return true, nil
	case types.SigBLS:
		apub := make([][]byte, len(sig.Signer))
		for i, id := range sig.Signer {
			addr, err := rm.GetPubKey(id, types.BLS)
			if err != nil {
				return false, err
			}
			apub[i] = addr.Bytes()
		}

		apk, err := bls.AggregatePublicKey(apub...)
		if err != nil {
			return false, err
		}
		ok, err := signature.Verify(apk, msg, sig.Data)
		if err != nil {
			return false, err
		}

		return ok, nil

	default:
		return false, xerrors.Errorf("sign type %d not supported", sig.Type)
	}
}

func (rm *RoleMgr) RoleSanityCheck(ctx context.Context, msg *tx.SignedMessage) (bool, error) {
	rm.Lock()
	defer rm.Unlock()

	if len(msg.Params) >= tx.MsgMaxLen {
		return false, xerrors.Errorf("msg length too long")
	}

	ri, err := rm.get(msg.From)
	if err != nil {
		return false, err
	}

	switch msg.Method {
	case tx.UpdateChalEpoch, tx.ConfirmPostIncome, tx.ConfirmDataOrder:
		// verift tx.From keeper
		if ri.Type != pb.RoleInfo_Keeper {
			return false, xerrors.Errorf("role type expected %d, got %d", pb.RoleInfo_Keeper, ri.Type)
		}
	case tx.CreateFs, tx.CreateBucket, tx.PreDataOrder, tx.AddDataOrder, tx.CommitDataOrder:
		// verify tx.From user
		if ri.Type != pb.RoleInfo_User {
			return false, xerrors.Errorf("role type expected %d, got %d", pb.RoleInfo_User, ri.Type)
		}
	case tx.SegmentProof, tx.SegmentFault, tx.SubDataOrder:
		// verify tx.From provider
		if ri.Type != pb.RoleInfo_Provider {
			return false, xerrors.Errorf("role type expected %d, got %d", pb.RoleInfo_Provider, ri.Type)
		}
	case tx.DataTxErr:
		return false, xerrors.Errorf("unsupported type")
	}

	return true, nil
}
