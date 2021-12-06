package minit

import (
	"context"
	"log"

	"github.com/gogo/protobuf/proto"
	"github.com/zeebo/blake3"

	"github.com/memoio/go-mefs-v2/lib/crypto/pdp"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	"github.com/memoio/go-mefs-v2/lib/crypto/signature"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/repo"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/submodule/connect/settle"
	"github.com/memoio/go-mefs-v2/submodule/network"
	"github.com/memoio/go-mefs-v2/submodule/wallet"
)

// init ops for mefs
func Create(ctx context.Context, r repo.Repo, password string) error {
	if _, _, err := network.GetSelfNetKey(r.KeyStore()); err != nil {
		return err
	}

	ri := new(pb.RoleInfo)

	if r.Config().Wallet.DefaultAddress != "" {
		return nil
	}

	w := wallet.New(password, r.KeyStore())

	log.Println("generating wallet key...")

	privkey, err := signature.GenerateKey(types.Secp256k1)
	if err != nil {
		return err
	}

	sBytes, err := privkey.Raw()
	if err != nil {
		return err
	}

	wki := &types.KeyInfo{
		Type:      types.Secp256k1,
		SecretKey: sBytes,
	}

	addr, err := w.WalletImport(ctx, wki)
	if err != nil {
		return err
	}

	log.Println("generated wallet: ", addr.String())

	ri.ChainVerifyKey = addr.Bytes()

	id, err := settle.GetRoleID(addr)
	if err != nil {
		return err
	}
	ri.ID = id
	ri.GroupID = settle.GetGroupID(id)

	cfg := r.Config()
	roleType := cfg.Identity.Role

	log.Println("generating bls key...")

	blsSeed := make([]byte, len(sBytes)+1)
	copy(blsSeed[:len(sBytes)], sBytes)
	blsSeed[len(sBytes)] = byte(types.BLS)
	blsByte := blake3.Sum256(blsSeed)
	blsKey := &types.KeyInfo{
		SecretKey: blsByte[:],
		Type:      types.BLS,
	}

	blsAddr, err := w.WalletImport(ctx, blsKey)
	if err != nil {
		return err
	}

	log.Println("genenrated bls: ", blsAddr.String())

	ri.BlsVerifyKey = blsAddr.Bytes()

	switch roleType {
	case "keeper":
		ri.Type = pb.RoleInfo_Keeper
	case "provider":
		ri.Type = pb.RoleInfo_Provider
	case "user":
		ri.Type = pb.RoleInfo_User

		blsSeed := make([]byte, len(sBytes)+1)
		copy(blsSeed[:len(sBytes)], sBytes)
		blsSeed[len(sBytes)] = byte(types.PDP)

		pdpKeySet, err := pdp.GenerateKeyWithSeed(pdpcommon.PDPV2, blsSeed)
		if err != nil {
			return err
		}

		// store pdp secretkey
		pdpsKey := types.KeyInfo{
			SecretKey: pdpKeySet.SecreteKey().Serialize(),
			Type:      types.PDP,
		}

		err = r.KeyStore().Put("pdp", password, pdpsKey)
		if err != nil {
			return err
		}

		ri.Extra = pdpKeySet.VerifyKey().Serialize()

		// store pdp pubkey and verify key
		log.Println("generated user bls pdp key")
	default:
		ri.Type = pb.RoleInfo_Unknown
		log.Println("type:", roleType)
	}

	// store roleinfo
	data, err := proto.Marshal(ri)
	if err != nil {
		return err
	}
	err = r.MetaStore().Put(store.NewKey(pb.MetaType_RoleInfoKey), data)
	if err != nil {
		return err
	}

	cfg.Wallet.DefaultAddress = addr.String()

	return nil
}
