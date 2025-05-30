package minit

import (
	"context"
	"encoding/hex"

	"github.com/ethereum/go-ethereum/common"
	"github.com/zeebo/blake3"

	"github.com/memoio/go-mefs-v2/lib/crypto/signature"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/repo"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/utils"
	"github.com/memoio/go-mefs-v2/submodule/network"
	"github.com/memoio/go-mefs-v2/submodule/wallet"
)

var logger = logging.Logger("init")

// init ops for mefs
func Create(ctx context.Context, r repo.Repo, password, sk string) error {
	if _, _, err := network.GetSelfNetKey(r.KeyStore()); err != nil {
		return err
	}

	if r.Config().Wallet.DefaultAddress != "" {
		return nil
	}

	w := wallet.New(password, r.KeyStore())

	var sBytes []byte
	if sk == "" {
		logger.Debug("generating wallet address...")

		privkey, err := signature.GenerateKey(types.Secp256k1)
		if err != nil {
			return err
		}

		sbytes, err := privkey.Raw()
		if err != nil {
			return err
		}
		sBytes = sbytes
	} else {
		sbytes, err := hex.DecodeString(sk)
		if err != nil {
			return err
		}

		sBytes = sbytes

	}
	wki := &types.KeyInfo{
		Type:      types.Secp256k1,
		SecretKey: sBytes,
	}

	addr, err := w.WalletImport(ctx, wki)
	if err != nil {
		return err
	}

	wa := common.BytesToAddress(utils.ToEthAddress(addr.Bytes()))

	if sk == "" {
		logger.Debug("generated wallet address: ", wa)
	} else {
		logger.Debug("import wallet address: ", wa)
	}

	logger.Debug("generating bls key...")

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

	logger.Debug("genenrated bls key: ", blsAddr.String())

	r.Config().Wallet.DefaultAddress = addr.String()

	// save config
	return r.ReplaceConfig(r.Config())
}
