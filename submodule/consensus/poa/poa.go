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

var logger = logging.Logger("hotstuff")

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

			tb := &tx.SignedBlock{
				RawBlock: tx.RawBlock{
					RawHeader: trh,
				},
				MultiSignature: types.NewMultiSignature(types.SigSecp256k1),
			}

			if trh.MinerID == m.localID {
				tb.MsgSet = m.app.Propose(trh)

				sig, err := m.RoleSign(m.ctx, m.localID, hs.CalcHash(tb.Hash().Bytes(), hs.PhaseCommit), types.SigSecp256k1)
				if err != nil {
					logger.Debug("create block signed invalid: ", err)
					continue
				}

				tb.MultiSignature.Add(m.localID, sig)
			}

			logger.Debugf("create new block at height: %d, slot: %d, now: %s, prev: %s, state now: %s, parent: %s, has message: %d", tb.Height, tb.Slot, tb.Hash().String(), tb.PrevID.String(), tb.Root.String(), tb.ParentRoot.String(), len(tb.Msgs))

			err = m.app.OnPropose(tb)
			if err != nil {
				logger.Debug("create block OnPropose: ", err)
				continue
			}

			err = m.app.OnViewDone(tb)
			if err != nil {
				logger.Debug("create block OnViewDone: ", err)
				continue
			}

			m.INetService.PublishTxBlock(m.ctx, tb)
		}
	}
}
