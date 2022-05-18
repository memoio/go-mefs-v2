package hotstuff

import (
	"context"
	"sync"
	"time"

	"go.opencensus.io/stats"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/build"
	hs "github.com/memoio/go-mefs-v2/lib/hotstuff"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	bcommon "github.com/memoio/go-mefs-v2/submodule/consensus/common"
	"github.com/memoio/go-mefs-v2/submodule/metrics"
)

var logger = logging.Logger("hotstuff")

type view struct {
	header tx.RawHeader
	txs    tx.MsgSet
	phase  hs.PhaseState

	members []uint64

	highQuorum      types.MultiSignature // hash(MsgNewView, Header)
	prepareQuorum   types.MultiSignature // hash(MsgPrepare, Header, Cmd)
	preCommitQuorum types.MultiSignature // hash(MsgPreCommit, Proposer Header, Cmd)
	commitQuorum    types.MultiSignature // hash(MsgCommit, Proposer Header, Cmd)

	createdAt time.Time
}

type HotstuffManager struct {
	api.IRole
	api.INetService

	lw  sync.Mutex
	ctx context.Context

	localID uint64

	curView *view

	// process
	app bcommon.ConsensusApp
}

func NewHotstuffManager(ctx context.Context, localID uint64, ir api.IRole, in api.INetService, a bcommon.ConsensusApp) *HotstuffManager {
	logger.Debug("create hotstuff consensus service")

	m := &HotstuffManager{
		IRole:       ir,
		INetService: in,
		ctx:         ctx,
		localID:     localID,
		app:         a,
		curView: &view{
			header: tx.RawHeader{},
		},
	}

	return m
}

func (hsm *HotstuffManager) MineBlock() {
	tc := time.NewTicker(1 * time.Second)
	defer tc.Stop()

	for {
		select {
		case <-hsm.ctx.Done():
			logger.Debug("mine block done")
			return
		case <-tc.C:
			slot := uint64(time.Now().Unix()-build.BaseTime) / build.SlotDuration
			if hsm.curView.header.Slot == slot {
				continue
			}

			err := hsm.NewView()
			if err != nil {
				logger.Debug("create block: ", err)
			}
		}
	}
}

func (hsm *HotstuffManager) HandleMessage(ctx context.Context, msg *hs.HotstuffMessage) error {
	logger.Debug("HandleMessage got: ", msg.From, msg.Type, msg.Data.RawHeader)

	hsm.lw.Lock()
	defer hsm.lw.Unlock()

	var err error
	switch msg.Type {
	case hs.MsgNewView:
		err = hsm.handleNewViewVoteMsg(msg)

	case hs.MsgPrepare:
		err = hsm.handlePrepareMsg(msg)
	case hs.MsgVotePrepare:
		err = hsm.handlePrepareVoteMsg(msg)

	case hs.MsgPreCommit:
		err = hsm.handlePreCommitMsg(msg)
	case hs.MsgVotePreCommit:
		err = hsm.handlePreCommitVoteMsg(msg)

	case hs.MsgCommit:
		err = hsm.handleCommitMsg(msg)
	case hs.MsgVoteCommit:
		return hsm.handleCommitVoteMsg(msg)

	case hs.MsgDecide:
		err = hsm.handleDecideMsg(msg)

	default:
		//logger.Warn("unknown hotstuff message type", msg.Type)
		return xerrors.Errorf("unknown hotstuff message type %d", msg.Type)
	}

	if err != nil {
		logger.Debug("HandleMessage err: ", msg.From, msg.Type, err)
	}

	return err
}

func (hsm *HotstuffManager) checkView(msg *hs.HotstuffMessage) error {
	if msg == nil {
		return xerrors.Errorf("checkView msg is nil")
	}

	slot := uint64(time.Now().Unix()-build.BaseTime) / build.SlotDuration
	if msg.Data.Slot < slot {
		return xerrors.Errorf("checkView msg from past views, expected %d, got %d", slot, msg.Data.Slot)
	}

	// update view
	if !hsm.curView.header.Hash().Equal(msg.Data.RawHeader.Hash()) {
		err := hsm.newView()
		if err != nil {
			return err
		}
	}

	if !hsm.curView.header.Hash().Equal(msg.Data.RawHeader.Hash()) {
		return xerrors.Errorf("checkView view not match, expected %w, got %w", hsm.curView.header, msg.Data.RawHeader)
	}

	has := false
	if len(hsm.curView.members) > 0 {
		for _, m := range hsm.curView.members {
			if m == msg.From {
				has = true
			}
		}
	}

	if !has {
		return xerrors.Errorf("checkView %d is not committee member", msg.From)
	}

	pType := hs.PhaseDecide
	switch msg.Type {
	case hs.MsgPrepare:
		pType = hs.PhaseNew
	case hs.MsgPreCommit:
		pType = hs.PhasePrepare
	case hs.MsgCommit:
		pType = hs.PhasePreCommit
	case hs.MsgDecide:
		pType = hs.PhaseCommit
	}

	// verify multisign
	if pType != hs.PhaseDecide {
		proposal := tx.RawBlock{
			RawHeader: msg.Data.RawHeader,
			MsgSet:    tx.MsgSet{},
		}
		var h []byte

		switch pType {
		// content is nil at phase new
		case hs.PhaseNew:
			h = hs.CalcHash(proposal.Hash().Bytes(), pType)
		case hs.PhasePrepare:
			proposal.MsgSet = msg.Data.MsgSet
			h = hs.CalcHash(proposal.Hash().Bytes(), pType)
		case hs.PhasePreCommit:
			proposal.MsgSet = hsm.curView.txs
			h = hs.CalcHash(proposal.Hash().Bytes(), pType)
		case hs.PhaseCommit:
			proposal.MsgSet = hsm.curView.txs
			h = hs.CalcHash(proposal.Hash().Bytes(), pType)
		}

		ok, err := hsm.RoleVerifyMulti(context.TODO(), h, msg.Quorum)
		if err != nil {
			return xerrors.Errorf("checkView %d quorum sign is invalid %w", pType, err)
		}

		if !ok {
			return xerrors.Errorf("checkView %d quorum sign is invalid", pType)
		}
	}

	pType = hs.PhaseDecide
	switch msg.Type {
	case hs.MsgNewView:
		pType = hs.PhaseNew
	case hs.MsgPrepare, hs.MsgVotePrepare:
		pType = hs.PhasePrepare
	case hs.MsgPreCommit, hs.MsgVotePreCommit:
		pType = hs.PhasePreCommit
	case hs.MsgCommit, hs.MsgVoteCommit:
		pType = hs.PhaseCommit
	}

	// verify data
	if pType != hs.PhaseDecide {
		ok, err := hsm.RoleVerify(context.TODO(), msg.From, hs.CalcHash(msg.Data.Hash().Bytes(), pType), msg.Sig)
		if err != nil {
			return xerrors.Errorf("checkView sign is invalid %w", err)
		}
		if !ok {
			return xerrors.Errorf("checkView sign is invalid")
		}
	}

	return nil
}

func (hsm *HotstuffManager) newView() error {
	slot := uint64(time.Now().Unix()-build.BaseTime) / build.SlotDuration

	// handle last one;
	if hsm.curView.header.Slot < slot {
		if hsm.curView.phase != hs.PhaseFinal && hsm.curView.commitQuorum.Len() > hsm.app.GetQuorumSize() {
			sb := &tx.SignedBlock{
				RawBlock: tx.RawBlock{
					RawHeader: hsm.curView.header,
					MsgSet:    hsm.curView.txs,
				},
				MultiSignature: hsm.curView.commitQuorum,
			}

			// apply last one
			err := hsm.app.OnViewDone(sb)
			if err != nil {
				return err
			}
			hsm.curView.phase = hs.PhaseFinal
		}
	} else if hsm.curView.header.Slot == slot {
		return nil
	} else {
		// time not syncd?
		return xerrors.Errorf("something badly happens, may be not syned")
	}

	// reset
	hsm.curView.phase = hs.PhaseInit
	hsm.curView.highQuorum = types.NewMultiSignature(types.SigBLS)
	hsm.curView.prepareQuorum = types.NewMultiSignature(types.SigBLS)
	hsm.curView.preCommitQuorum = types.NewMultiSignature(types.SigBLS)
	hsm.curView.commitQuorum = types.NewMultiSignature(types.SigBLS)
	hsm.curView.txs = tx.MsgSet{}

	hsm.curView.createdAt = time.Now()

	// get newest state: block height, leader, parent
	rh, err := hsm.app.CreateBlockHeader()
	if err != nil {
		return err
	}

	hsm.curView.header = rh
	hsm.curView.members = hsm.app.GetMembers()

	has := false
	if len(hsm.curView.members) > 0 {
		for _, m := range hsm.curView.members {
			if m == hsm.localID {
				has = true
			}
		}
	}

	if !has && len(hsm.curView.members) > 0 {
		return xerrors.Errorf("%d is not committee member", hsm.localID)
	}

	logger.Debug("create new view: ", hsm.localID, hsm.curView.header, hsm.curView.members)

	return nil
}

// when time is up, enter into new view
// replicas send to leader
func (hsm *HotstuffManager) NewView() error {
	hsm.lw.Lock()
	defer hsm.lw.Unlock()

	err := hsm.newView()
	if err != nil {
		return err
	}

	if hsm.curView.phase != hs.PhaseInit {
		return xerrors.Errorf("phase is wrong")
	}

	if hsm.curView.header.Height == 0 {
		hsm.curView.phase = hs.PhaseFinal

		txs, err := hsm.app.Propose(hsm.curView.header)
		if err != nil {
			logger.Debug("propose fail: ", err)
			return err
		}

		mid := hsm.localID
		for _, msg := range txs.Msgs {
			if msg.From < mid {
				mid = msg.From
			}
		}

		if mid != hsm.localID {
			return nil
		}

		hsm.curView.txs = txs
		sp := &tx.SignedBlock{
			RawBlock: tx.RawBlock{
				RawHeader: hsm.curView.header,
				MsgSet:    hsm.curView.txs,
			},
			MultiSignature: types.NewMultiSignature(types.SigBLS),
		}

		sig, err := hsm.RoleSign(context.TODO(), hsm.localID, hs.CalcHash(sp.Hash().Bytes(), hs.PhaseCommit), types.SigBLS)
		if err != nil {
			return err
		}

		sp.MultiSignature.Add(hsm.localID, sig)

		hsm.app.OnViewDone(sp)
		return nil
	}

	// replica send out to leader
	sp := tx.RawBlock{
		RawHeader: hsm.curView.header,
		MsgSet:    tx.MsgSet{},
	}

	hsm.curView.phase = hs.PhaseNew

	sig, err := hsm.RoleSign(context.TODO(), hsm.localID, hs.CalcHash(sp.Hash().Bytes(), hsm.curView.phase), types.SigBLS)
	if err != nil {
		return err
	}

	if hsm.curView.header.MinerID == hsm.localID {
		logger.Debug("leader new view: ", hsm.curView.header.Slot, hsm.curView.header.Height)
		err = hsm.curView.highQuorum.Add(hsm.localID, sig)
		if err != nil {
			return err
		}
		stats.Record(hsm.ctx, metrics.TxBlockCreateExpected.M(1))
	} else {
		hm := &hs.HotstuffMessage{
			From: hsm.localID,
			Type: hs.MsgNewView,
			Data: sp,
			Sig:  sig,
		}

		// send hm message out
		err = hsm.PublishHsMsg(hsm.ctx, hm)
		if err != nil {
			logger.Debugf("publish msg err %s", err)
		}
	}

	return nil
}

// leader handle new view msg from replicas
func (hsm *HotstuffManager) handleNewViewVoteMsg(msg *hs.HotstuffMessage) error {

	// not handle it
	if hsm.localID != msg.Data.MinerID {
		return nil
	}

	logger.Debug("leader check view: ", msg.Data.Slot, msg.From, msg.Type)

	err := hsm.checkView(msg)
	if err != nil {
		return err
	}

	if hsm.curView.phase == hs.PhaseInit {
		sp := tx.RawBlock{
			RawHeader: hsm.curView.header,
			MsgSet:    tx.MsgSet{},
		}

		hsm.curView.phase = hs.PhaseNew

		sig, err := hsm.RoleSign(context.TODO(), hsm.localID, hs.CalcHash(sp.Hash().Bytes(), hsm.curView.phase), types.SigBLS)
		if err != nil {
			return err
		}

		logger.Debug("leader new view: ", hsm.curView.header.Slot, hsm.curView.header.Height)
		err = hsm.curView.highQuorum.Add(hsm.localID, sig)
		if err != nil {
			return err
		}
		stats.Record(hsm.ctx, metrics.TxBlockCreateExpected.M(1))
	}

	if hsm.curView.phase != hs.PhaseNew {
		return xerrors.Errorf("phase state wrong, expected %d, got %d", hs.PhaseNew, hsm.curView.phase)
	}

	logger.Debug("leader add sign: ", msg.Data.Slot, msg.From, msg.Type)

	err = hsm.curView.highQuorum.Add(msg.From, msg.Sig)
	if err != nil {
		return err
	}

	logger.Debugf("leader %d get new quorum %d, %d, %d", hsm.curView.header.Slot, hsm.curView.highQuorum.Len(), hsm.localID, hsm.curView.header.Height)

	if hsm.curView.highQuorum.Len() >= hsm.app.GetQuorumSize() {
		return hsm.tryPropose()
	}

	return nil
}

// lead send prepare msg
func (hsm *HotstuffManager) tryPropose() error {
	if hsm.curView.phase != hs.PhaseNew {
		return xerrors.Errorf("phase state error")
	}

	logger.Debugf("leader enter into propose %d, %d, %d", hsm.curView.header.Slot, hsm.localID, hsm.curView.header.Height)

	hsm.curView.phase = hs.PhasePrepare

	txs, err := hsm.app.Propose(hsm.curView.header)
	if err != nil {
		logger.Debug("leader propose fail: ", err)
		return err
	}

	hsm.curView.txs = txs

	sp := tx.RawBlock{
		RawHeader: hsm.curView.header,
		MsgSet:    hsm.curView.txs,
	}

	sig, err := hsm.RoleSign(context.TODO(), hsm.localID, hs.CalcHash(sp.Hash().Bytes(), hsm.curView.phase), types.SigBLS)
	if err != nil {
		return err
	}

	hsm.curView.prepareQuorum.Add(hsm.localID, sig)

	hm := &hs.HotstuffMessage{
		From:   hsm.localID,
		Type:   hs.MsgPrepare,
		Data:   sp,
		Sig:    sig,
		Quorum: hsm.curView.highQuorum,
	}

	logger.Debugf("leader propose publish %d, %d, %d", hsm.curView.header.Slot, hsm.localID, hsm.curView.header.Height)

	err = hsm.PublishHsMsg(hsm.ctx, hm)
	if err != nil {
		logger.Debugf("publish msg err %s", err)
	}

	return nil
}

// replica response prepare msg
func (hsm *HotstuffManager) handlePrepareMsg(msg *hs.HotstuffMessage) error {
	// leader not handle it
	if hsm.localID == msg.Data.MinerID {
		return nil
	}

	err := hsm.checkView(msg)
	if err != nil {
		return err
	}

	if hsm.curView.header.MinerID != msg.From {
		return nil
	}

	// View is created in 'checkView' before 'NewView'.
	if hsm.curView.phase == hs.PhaseInit {
		// No need to sign and publish msg for view-change, only update the phase.
		hsm.curView.phase = hs.PhaseNew
	}

	if hsm.curView.phase != hs.PhaseNew {
		return xerrors.Errorf("phase state wrong, expected %d, got %d", hs.PhaseNew, hsm.curView.phase)
	}

	// validate propose
	sb := &tx.SignedBlock{
		RawBlock: msg.Data,
	}
	err = hsm.app.OnPropose(sb)
	if err != nil {
		return err
	}

	hsm.curView.phase = hs.PhasePrepare
	hsm.curView.highQuorum = msg.Quorum

	hsm.curView.txs = msg.Data.MsgSet

	msg.From = hsm.localID
	sig, err := hsm.RoleSign(context.TODO(), hsm.localID, hs.CalcHash(msg.Data.Hash().Bytes(), hsm.curView.phase), types.SigBLS)
	if err != nil {
		return err
	}
	msg.Sig = sig
	msg.Quorum = types.NewMultiSignature(types.SigBLS)
	msg.Type = hs.MsgVotePrepare

	// send to leader
	err = hsm.PublishHsMsg(hsm.ctx, msg)
	if err != nil {
		logger.Debugf("publish msg err %s", err)
	}

	return nil
}

// leader handle prepare response
// leader send precommit
func (hsm *HotstuffManager) handlePrepareVoteMsg(msg *hs.HotstuffMessage) error {
	if hsm.localID != msg.Data.MinerID {
		return nil
	}

	err := hsm.checkView(msg)
	if err != nil {
		return err
	}

	if hsm.curView.phase != hs.PhasePrepare {
		return xerrors.Errorf("phase state wrong, expected %d, got %d", hs.PhasePrepare, hsm.curView.phase)
	}

	err = hsm.curView.prepareQuorum.Add(msg.From, msg.Sig)
	if err != nil {
		return err
	}

	logger.Debugf("leader %d get prepare quorum %d, %d, %d", hsm.curView.header.Slot, hsm.curView.prepareQuorum.Len(), hsm.localID, hsm.curView.header.Height)

	if hsm.curView.prepareQuorum.Len() >= hsm.app.GetQuorumSize() {
		return hsm.tryPreCommit()
	}

	return nil
}

// leader send precommit message
func (hsm *HotstuffManager) tryPreCommit() error {
	if hsm.curView.phase != hs.PhasePrepare {
		return xerrors.Errorf("phase state error")
	}

	logger.Debugf("leader enter into precommit %d %d %d", hsm.curView.header.Slot, hsm.localID, hsm.curView.header.Height)

	hsm.curView.phase = hs.PhasePreCommit

	sp := tx.RawBlock{
		RawHeader: hsm.curView.header,
		MsgSet:    hsm.curView.txs,
	}

	sig, err := hsm.RoleSign(context.TODO(), hsm.localID, hs.CalcHash(sp.Hash().Bytes(), hsm.curView.phase), types.SigBLS)
	if err != nil {
		return err
	}

	hsm.curView.preCommitQuorum.Add(hsm.localID, sig)

	hm := &hs.HotstuffMessage{
		From:   hsm.localID,
		Type:   hs.MsgPreCommit,
		Data:   sp,
		Sig:    sig,
		Quorum: hsm.curView.prepareQuorum,
	}

	err = hsm.PublishHsMsg(hsm.ctx, hm)
	if err != nil {
		logger.Debugf("publish msg err %s", err)
	}

	return nil
}

// replica handle precommit message
func (hsm *HotstuffManager) handlePreCommitMsg(msg *hs.HotstuffMessage) error {
	if hsm.localID == msg.Data.MinerID {
		return nil
	}

	err := hsm.checkView(msg)
	if err != nil {
		return err
	}

	if hsm.curView.header.MinerID != msg.From {
		return nil
	}

	if hsm.curView.phase != hs.PhasePrepare {
		return xerrors.Errorf("phase state wrong, expected %d, got %d", hs.PhasePrepare, hsm.curView.phase)
	}

	hsm.curView.phase = hs.PhasePreCommit

	hsm.curView.prepareQuorum = msg.Quorum

	msg.From = hsm.localID
	sig, err := hsm.RoleSign(context.TODO(), hsm.localID, hs.CalcHash(msg.Data.Hash().Bytes(), hsm.curView.phase), types.SigBLS)
	if err != nil {
		return err
	}
	msg.Sig = sig
	msg.Quorum = types.NewMultiSignature(types.SigBLS)
	msg.Type = hs.MsgVotePreCommit

	// send to leader
	err = hsm.PublishHsMsg(hsm.ctx, msg)
	if err != nil {
		logger.Debugf("publish msg err %s", err)
	}

	return nil
}

// leader handle precommit response
func (hsm *HotstuffManager) handlePreCommitVoteMsg(msg *hs.HotstuffMessage) error {
	if hsm.localID != msg.Data.MinerID {
		return nil
	}

	err := hsm.checkView(msg)
	if err != nil {
		return err
	}

	if hsm.curView.phase != hs.PhasePreCommit {
		return xerrors.Errorf("phase state wrong, expected %d, got %d", hs.PhasePreCommit, hsm.curView.phase)
	}

	err = hsm.curView.preCommitQuorum.Add(msg.From, msg.Sig)
	if err != nil {
		return err
	}

	logger.Debugf("leader %d get precommit quorum %d, %d, %d", hsm.curView.header.Slot, hsm.curView.preCommitQuorum.Len(), hsm.localID, hsm.curView.header.Height)

	if hsm.curView.preCommitQuorum.Len() >= hsm.app.GetQuorumSize() {
		return hsm.tryCommit()
	}
	return nil
}

// leader send commit
func (hsm *HotstuffManager) tryCommit() error {
	if hsm.curView.phase != hs.PhasePreCommit {
		return xerrors.Errorf("phase state error")
	}

	logger.Debugf("leader enter into commit %d %d %d", hsm.curView.header.Slot, hsm.localID, hsm.curView.header.Height)

	hsm.curView.phase = hs.PhaseCommit

	sp := tx.RawBlock{
		RawHeader: hsm.curView.header,
		MsgSet:    hsm.curView.txs,
	}

	sig, err := hsm.RoleSign(context.TODO(), hsm.localID, hs.CalcHash(sp.Hash().Bytes(), hsm.curView.phase), types.SigBLS)
	if err != nil {
		return err
	}

	hsm.curView.commitQuorum.Add(hsm.localID, sig)

	hm := &hs.HotstuffMessage{
		From:   hsm.localID,
		Type:   hs.MsgCommit,
		Data:   sp,
		Sig:    sig,
		Quorum: hsm.curView.preCommitQuorum,
	}

	err = hsm.PublishHsMsg(hsm.ctx, hm)
	if err != nil {
		logger.Debugf("publish msg err %s", err)
	}

	return nil
}

// replica handle commit message
func (hsm *HotstuffManager) handleCommitMsg(msg *hs.HotstuffMessage) error {
	if hsm.localID == msg.Data.MinerID {
		return nil
	}

	err := hsm.checkView(msg)
	if err != nil {
		return err
	}

	if hsm.curView.phase != hs.PhasePreCommit {
		return xerrors.Errorf("phase state wrong, expected %d, got %d", hs.PhasePreCommit, hsm.curView.phase)
	}

	hsm.curView.phase = hs.PhaseCommit

	hsm.curView.preCommitQuorum = msg.Quorum

	msg.From = hsm.localID
	sig, err := hsm.RoleSign(context.TODO(), hsm.localID, hs.CalcHash(msg.Data.Hash().Bytes(), hsm.curView.phase), types.SigBLS)
	if err != nil {
		return err
	}
	msg.Sig = sig
	msg.Quorum = types.NewMultiSignature(types.SigBLS)
	msg.Type = hs.MsgVoteCommit

	err = hsm.PublishHsMsg(hsm.ctx, msg)
	if err != nil {
		logger.Debugf("publish msg err %s", err)
	}

	return nil
}

// leader handle commit response
func (hsm *HotstuffManager) handleCommitVoteMsg(msg *hs.HotstuffMessage) error {
	if hsm.localID != msg.Data.MinerID {
		return nil
	}

	err := hsm.checkView(msg)
	if err != nil {
		return err
	}

	if hsm.curView.phase != hs.PhaseCommit {
		return xerrors.Errorf("phase state wrong, expected %d, got %d", hs.PhaseCommit, hsm.curView.phase)
	}

	err = hsm.curView.commitQuorum.Add(msg.From, msg.Sig)
	if err != nil {
		return err
	}

	logger.Debugf("leader %d get commit quorum %d, %d, %d", hsm.curView.header.Slot, hsm.curView.commitQuorum.Len(), hsm.localID, hsm.curView.header.Height)

	if hsm.curView.commitQuorum.Len() >= hsm.app.GetQuorumSize() {
		return hsm.tryDecide()
	}
	return nil
}

// leader sends decide message
func (hsm *HotstuffManager) tryDecide() error {
	if hsm.curView.phase != hs.PhaseCommit {
		return xerrors.Errorf("phase state error")
	}

	logger.Debugf("leader enter into decide %d %d %d", hsm.curView.header.Slot, hsm.localID, hsm.curView.header.Height)

	hsm.curView.phase = hs.PhaseDecide

	sp := tx.RawBlock{
		RawHeader: hsm.curView.header,
		MsgSet:    hsm.curView.txs,
	}

	sig, err := hsm.RoleSign(context.TODO(), hsm.localID, hs.CalcHash(sp.Hash().Bytes(), hsm.curView.phase), types.SigBLS)
	if err != nil {
		return err
	}

	hm := &hs.HotstuffMessage{
		From:   hsm.localID,
		Type:   hs.MsgDecide,
		Data:   sp,
		Sig:    sig,
		Quorum: hsm.curView.commitQuorum,
	}

	// send hm message out
	err = hsm.PublishHsMsg(hsm.ctx, hm)
	if err != nil {
		logger.Debugf("publish msg err %s", err)
	}

	// apply
	sb := &tx.SignedBlock{
		RawBlock:       sp,
		MultiSignature: hsm.curView.commitQuorum,
	}

	err = hsm.app.OnViewDone(sb)
	if err != nil {
		return err
	}

	hsm.curView.phase = hs.PhaseFinal

	stats.Record(hsm.ctx, metrics.TxBlockCreateSuccess.M(1))

	return nil
}

// replica handle decide message
func (hsm *HotstuffManager) handleDecideMsg(msg *hs.HotstuffMessage) error {
	if hsm.localID == msg.Data.MinerID {
		return nil
	}

	err := hsm.checkView(msg)
	if err != nil {
		return err
	}

	if hsm.curView.phase != hs.PhaseCommit {
		return xerrors.Errorf("phase state wrong, expected %d, got %d", hs.PhaseCommit, hsm.curView.phase)
	}

	hsm.curView.phase = hs.PhaseDecide

	hsm.curView.commitQuorum = msg.Quorum

	sb := &tx.SignedBlock{
		RawBlock: tx.RawBlock{
			RawHeader: hsm.curView.header,
			MsgSet:    hsm.curView.txs,
		},
		MultiSignature: hsm.curView.commitQuorum,
	}

	err = hsm.app.OnViewDone(sb)
	if err != nil {
		return err
	}

	hsm.curView.phase = hs.PhaseFinal

	return nil
}
