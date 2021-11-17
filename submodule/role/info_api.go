package role

import (
	"context"

	"github.com/memoio/go-mefs-v2/api"
	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12_381"
	"github.com/memoio/go-mefs-v2/lib/crypto/signature"
	"github.com/memoio/go-mefs-v2/lib/crypto/signature/secp256k1"
	mSign "github.com/memoio/go-mefs-v2/lib/multiSign"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
)

var _ api.IRole = &roleAPI{}

type roleAPI struct {
	*RoleMgr
}

func (rm *RoleMgr) RoleSelf(ctx context.Context) (pb.RoleInfo, error) {
	rm.RLock()
	defer rm.RUnlock()

	ri, ok := rm.infos[rm.roleID]
	if ok {
		return ri, nil
	}
	return pb.RoleInfo{}, ErrNotFound
}

func (rm *RoleMgr) RoleGet(ctx context.Context, id uint64) (pb.RoleInfo, error) {
	rm.RLock()
	defer rm.RUnlock()
	ri, ok := rm.infos[id]
	if ok {
		return ri, nil
	}
	return pb.RoleInfo{}, ErrNotFound
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
		return nil, ErrNotFound
	}
}

func (rm *RoleMgr) RoleSign(ctx context.Context, msg []byte, typ types.SigType) (types.Signature, error) {
	ts := types.Signature{
		Type: typ,
	}

	switch typ {
	case types.SigSecp256k1:
		sig, err := rm.WalletSign(rm.ctx, rm.localAddr, msg)
		if err != nil {
			return ts, err
		}
		ts.Data = sig
	case types.SigBLS:
		sig, err := rm.WalletSign(rm.ctx, rm.blsAddr, msg)
		if err != nil {
			return ts, err
		}
		ts.Data = sig
	default:
		return ts, ErrNotFound
	}

	//logger.Debug("sign message:", base64.RawStdEncoding.EncodeToString(rm.localAddr.Bytes()), base64.RawStdEncoding.EncodeToString(msg), base64.RawStdEncoding.EncodeToString(ts.Data))

	return ts, nil
}

func (rm *RoleMgr) RoleVerify(ctx context.Context, id uint64, msg []byte, sig types.Signature) (bool, error) {
	var pubByte []byte
	switch sig.Type {
	case types.SigSecp256k1:
		pubByte = rm.GetPubKey(id)
	case types.SigBLS:
		pubByte = rm.GetBlsPubKey(id)
	default:
		return false, ErrNotFound
	}

	if len(pubByte) == 0 {
		logger.Warn("local has no pubkey for:", id)
		return false, ErrNotFound
	}

	//logger.Debug("verify sign message:", base64.RawStdEncoding.EncodeToString(pubByte), base64.RawStdEncoding.EncodeToString(msg), base64.RawStdEncoding.EncodeToString(sig.Data))

	ok, err := signature.Verify(pubByte, msg, sig.Data)
	if err != nil {
		return false, err
	}

	return ok, nil
}

func (rm *RoleMgr) RoleVerifyMulti(ctx context.Context, msg []byte, sig mSign.MultiSignature) (bool, error) {
	switch sig.Type {
	case types.SigSecp256k1:
		for i, id := range sig.Signer {
			if len(sig.Data) < (i+1)*secp256k1.SignatureSize {
				return false, ErrNotFound
			}
			pubByte := rm.GetPubKey(id)
			sign := sig.Data[i*secp256k1.SignatureSize : (i+1)*secp256k1.SignatureSize]
			ok, err := signature.Verify(pubByte, msg, sign)
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
			if len(sig.Data) < (i+1)*secp256k1.SignatureSize {
				return false, ErrNotFound
			}
			pubByte := rm.GetPubKey(id)
			if len(pubByte) == 0 {
				return false, ErrNotFound
			}
			apub[i] = pubByte
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
		return false, ErrNotFound
	}
}
