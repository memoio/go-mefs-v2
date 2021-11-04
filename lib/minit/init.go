package minit

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gogo/protobuf/proto"
	"github.com/zeebo/blake3"

	pdpv2 "github.com/memoio/go-mefs-v2/lib/crypto/pdp/version2"
	"github.com/memoio/go-mefs-v2/lib/crypto/signature"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/repo"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/submodule/connect/settle"
	"github.com/memoio/go-mefs-v2/submodule/network"
	"github.com/memoio/go-mefs-v2/submodule/wallet"
)

// init ops for mefs
func Init(ctx context.Context, r repo.Repo, password string) error {

	if _, _, err := network.GetSelfNetKey(r.KeyStore()); err != nil {
		return err
	}

	ri := new(pb.RoleInfo)

	if r.Config().Wallet.DefaultAddress != "" {
		return nil
	}

	w := wallet.New(password, r.KeyStore())

	fmt.Println("generating wallet address...")

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

	fmt.Println("wallet addr: ", addr.String())

	ri.ChainVerifyKey = addr.Bytes()

	id, err := settle.GetRoleID(addr)
	if err != nil {
		return err
	}
	ri.ID = id
	ri.GroupID = settle.GetGroupID(id)

	cfg := r.Config()
	roleType := cfg.Identity.Role

	switch roleType {
	case "keeper", "provider":
		if roleType == "keeper" {
			ri.Type = pb.RoleInfo_Keeper
		} else {
			ri.Type = pb.RoleInfo_Provider
		}

		blsByte := blake3.Sum256(sBytes)
		blsKey := &types.KeyInfo{
			SecretKey: blsByte[:],
			Type:      types.BLS,
		}

		blsAddr, err := w.WalletImport(ctx, blsKey)
		if err != nil {
			return err
		}

		ri.BlsVerifyKey = blsAddr.Bytes()

		fmt.Println("bls addr: ", blsAddr.String())
	case "user":
		ri.Type = pb.RoleInfo_User
		pdpKeySet, err := pdpv2.GenKeySetWithSeed(sBytes, pdpv2.SCount)
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

		ri.BlsVerifyKey = pdpKeySet.VerifyKey().Serialize()

		// store pdp pubkey and verify key

		r.MetaStore().Put([]byte(strconv.Itoa(int(pb.MetaType_PDPProveKey))), pdpKeySet.PublicKey().Serialize())
		r.MetaStore().Put([]byte(strconv.Itoa(int(pb.MetaType_PDPVerifyKey))), pdpKeySet.VerifyKey().Serialize())
		fmt.Println("generate bls pdp key ")
	default:
		ri.Type = pb.RoleInfo_Unknown
		fmt.Println("unsupported type:", roleType)
	}

	// store roleinfo
	data, _ := proto.Marshal(ri)
	r.MetaStore().Put([]byte(strconv.Itoa(int(pb.MetaType_RoleInfoKey))), data)

	cfg.Wallet.DefaultAddress = addr.String()

	return nil
}
