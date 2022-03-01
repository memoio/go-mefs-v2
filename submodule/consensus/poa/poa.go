package poa

import (
	"context"
	"time"

	"github.com/memoio/go-mefs-v2/api"
	hs "github.com/memoio/go-mefs-v2/lib/hotstuff"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	bcommon "github.com/memoio/go-mefs-v2/submodule/consensus/common"
)

var logger = logging.Logger("poa")

// one keeper
type PoAManager struct {
	api.IRole
	api.INetService

	ctx context.Context

	localID uint64

	// process
	app bcommon.ConsensusApp
}

func NewPoAManager(ctx context.Context, localID uint64, ir api.IRole, in api.INetService, a bcommon.ConsensusApp) *PoAManager {
	logger.Debug("create poa consensus service")

	m := &PoAManager{
		IRole:       ir,
		INetService: in,
		ctx:         ctx,
		localID:     localID,
		app:         a,
	}

	return m
}

func (m *PoAManager) MineBlock() {
	tc := time.NewTicker(1 * time.Second)
	defer tc.Stop()

	for {
		select {
		case <-m.ctx.Done():
			logger.Debug("mine block done")
			return
		case <-tc.C:
			trh, err := m.app.CreateBlockHeader()
			if err != nil {
				logger.Debug("create block header: ", err)
				continue
			}

			nt := time.Now()

			tb := &tx.SignedBlock{
				RawBlock: tx.RawBlock{
					RawHeader: trh,
				},
				MultiSignature: types.NewMultiSignature(types.SigSecp256k1),
			}

			if trh.MinerID == m.localID {
				txs, err := m.app.Propose(trh)
				if err != nil {
					continue
				}
				tb.MsgSet = txs
			}

			logger.Debugf("create block propose cost %s", time.Since(nt))

			err = m.app.OnPropose(tb)
			if err != nil {
				logger.Debug("create block OnPropose: ", err)
				continue
			}

			logger.Debugf("create block OnPropose cost %s", time.Since(nt))

			if trh.MinerID == m.localID {
				sig, err := m.RoleSign(m.ctx, m.localID, hs.CalcHash(tb.Hash().Bytes(), hs.PhaseCommit), types.SigSecp256k1)
				if err != nil {
					logger.Debug("create block signed invalid: ", err)
					continue
				}

				tb.MultiSignature.Add(m.localID, sig)
			}

			logger.Debugf("create block sign cost %s", time.Since(nt))

			err = m.app.OnViewDone(tb)
			if err != nil {
				logger.Debug("create block OnViewDone: ", err)
				continue
			}

			m.INetService.PublishTxBlock(m.ctx, tb)
		}
	}
}
