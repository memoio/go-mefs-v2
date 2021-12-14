package hotstuff

import (
	"context"
	"fmt"
	"log"
	"time"

	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/build"
	hs "github.com/memoio/go-mefs-v2/lib/hotstuff"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	bcommon "github.com/memoio/go-mefs-v2/submodule/consensus/common"
)

var logger = logging.Logger("hotstuff")

type HotstuffManager struct {
	api.IRole
	api.INetService

	ctx context.Context

	localID uint64

	curView *view

	// process
	app bcommon.ConsensusApp
}

func NewHotstuffManager(ctx context.Context, localID uint64, ir api.IRole, in api.INetService, a bcommon.ConsensusApp) *HotstuffManager {
	m := &HotstuffManager{
		IRole:       ir,
		INetService: in,
		ctx:         ctx,
		localID:     localID,
		app:         a,
		curView: &view{
			header: tx.RawHeader{
				Version: 1,
			},
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
			err := hsm.NewView()
			if err != nil {
				logger.Debug("create block: ", err)
			}
		}
	}
}

func (hsm *HotstuffManager) HandleMessage(ctx context.Context, msg *hs.HotstuffMessage) error {
	switch msg.Type {
	case hs.MsgNewView:
		return hsm.handleNewViewVoteMsg(msg)

	case hs.MsgPrepare:
		return hsm.handlePrepareMsg(msg)
	case hs.MsgVotePrepare:
		return hsm.handlePrepareVoteMsg(msg)

	case hs.MsgPreCommit:
		return hsm.handlePreCommitMsg(msg)
	case hs.MsgVotePreCommit:
		return hsm.handlePreCommitVoteMsg(msg)

	case hs.MsgCommit:
		return hsm.handleCommitMsg(msg)
	case hs.MsgVoteCommit:
		return hsm.handleCommitVoteMsg(msg)

	case hs.MsgDecide:
		return hsm.handleDecideMsg(msg)

	default:
		fmt.Println("unknown hotstuff message type", msg.Type)
		return xerrors.Errorf("unknown hotstuff message type %d", msg.Type)
	}
}

type view struct {
	header tx.RawHeader
	txs    tx.MsgSet

	phase hs.PhaseState

	highQuorum      types.MultiSignature // hash(MsgNewView, Header)
	prepareQuorum   types.MultiSignature // hash(MsgPrepare, Header, Cmd)
	preCommitQuorum types.MultiSignature // hash(MsgPreCommit, Proposer Header, Cmd)
	commitQuorum    types.MultiSignature // hash(MsgCommit, Proposer Header, Cmd)

	createdAt time.Time
}

func (hsm *HotstuffManager) checkView(msg *hs.HotstuffMessage) error {
	if msg == nil {
		return xerrors.Errorf("msg is nil")
	}

	slot := uint64(time.Now().Unix()-build.BaseTime) / build.SlotDuration
	if msg.Data.Slot < slot {
		return xerrors.Errorf("msg from past views")
	}

	if hsm.curView != nil {
		if hsm.curView.header.Slot != slot {
			err := hsm.newView()
			if err != nil {
				return err
			}
		}

		if !hsm.curView.header.Hash().Equal(msg.Data.RawHeader.Hash()) {
			return xerrors.Errorf("view not match")
		}
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
			return err
		}

		if !ok {
			return xerrors.Errorf("sign is invalid")
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
			return err
		}
		if !ok {
			return xerrors.Errorf("sign is invalid")
		}
	}

	return xerrors.Errorf("empty curView")
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
	hsm.curView.highQuorum = types.NewMultiSignature(types.SigBLS)
	hsm.curView.prepareQuorum = types.NewMultiSignature(types.SigBLS)
	hsm.curView.preCommitQuorum = types.NewMultiSignature(types.SigBLS)
	hsm.curView.commitQuorum = types.NewMultiSignature(types.SigBLS)

	hsm.curView.phase = hs.PhaseNew

	hsm.curView.createdAt = time.Now()

	// get newest state: block height, leader, parent
	rh, err := hsm.app.CreateBlockHeader()
	if err != nil {
		return err
	}

	hsm.curView.header = rh

	log.Println("create new view", "leader", hsm.curView.header.MinerID, "view slot", hsm.curView.header.Slot)

	return nil
}

// when time is up, enter into new view
// replicas send to leader
func (hsm *HotstuffManager) NewView() error {
	err := hsm.newView()
	if err != nil {
		return err
	}

	if hsm.curView.header.MinerID == hsm.localID {
		return nil
	}

	hsm.curView.phase = hs.PhaseNew

	// replica send out to leader
	sp := tx.RawBlock{
		RawHeader: hsm.curView.header,
		MsgSet:    tx.MsgSet{},
	}

	sig, err := hsm.RoleSign(context.TODO(), hsm.localID, hs.CalcHash(sp.Hash().Bytes(), hsm.curView.phase), types.SigBLS)
	if err != nil {
		return err
	}

	hm := &hs.HotstuffMessage{
		From: hsm.localID,
		Type: hs.MsgNewView,
		Data: sp,
		Sig:  sig,
	}

	err = hsm.curView.highQuorum.Add(hsm.localID, sig)
	if err != nil {
		return err
	}

	// todo: send hm message out
	hsm.PublishHsMsg(hsm.ctx, hm)

	return nil
}

// leader handle new view msg from replicas
func (hsm *HotstuffManager) handleNewViewVoteMsg(msg *hs.HotstuffMessage) error {
	log.Println("handleNewViewMsg got new view message", "from", msg.From, "view", msg.Data.RawHeader)

	// not handle it
	if hsm.localID != msg.Data.MinerID {
		return nil
	}

	err := hsm.checkView(msg)
	if err != nil {
		return err
	}

	if hsm.curView.phase != hs.PhaseNew {
		return xerrors.Errorf("phase state wrong")
	}

	err = hsm.curView.highQuorum.Add(msg.From, msg.Sig)
	if err != nil {
		return err
	}

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

	hsm.curView.phase = hs.PhasePrepare

	txs, err := hsm.app.Propose(hsm.curView.header)
	if err != nil {
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

	hm := &hs.HotstuffMessage{
		From:   hsm.localID,
		Type:   hs.MsgPrepare,
		Data:   sp,
		Sig:    sig,
		Quorum: hsm.curView.highQuorum,
	}

	// todo: send hm message out
	hsm.PublishHsMsg(hsm.ctx, hm)

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

	if hsm.curView.phase != hs.PhaseNew {
		return xerrors.Errorf("phase state wrong")
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
	hsm.PublishHsMsg(hsm.ctx, msg)

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
		return xerrors.Errorf("phase state wrong")
	}

	err = hsm.curView.prepareQuorum.Add(msg.From, msg.Sig)
	if err != nil {
		return err
	}

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

	// todo: send hm message out
	hsm.PublishHsMsg(hsm.ctx, hm)

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

	if hsm.curView.phase != hs.PhasePrepare {
		return xerrors.Errorf("phase state wrong")
	}

	hsm.curView.phase = hs.PhasePreCommit

	msg.From = hsm.localID
	sig, err := hsm.RoleSign(context.TODO(), hsm.localID, hs.CalcHash(msg.Data.Hash().Bytes(), hsm.curView.phase), types.SigBLS)
	if err != nil {
		return err
	}
	msg.Sig = sig
	msg.Quorum = types.NewMultiSignature(types.SigBLS)
	msg.Type = hs.MsgVotePreCommit

	// send to leader
	hsm.PublishHsMsg(hsm.ctx, msg)

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
		return xerrors.Errorf("phase state wrong")
	}

	err = hsm.curView.preCommitQuorum.Add(msg.From, msg.Sig)
	if err != nil {
		return err
	}

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

	hsm.curView.phase = hs.PhaseCommit

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
		Type:   hs.MsgCommit,
		Data:   sp,
		Sig:    sig,
		Quorum: hsm.curView.preCommitQuorum,
	}

	// todo: send hm message out
	hsm.PublishHsMsg(hsm.ctx, hm)

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
		return xerrors.Errorf("phase state wrong")
	}

	hsm.curView.phase = hs.PhaseCommit

	hsm.curView.prepareQuorum = msg.Quorum

	msg.From = hsm.localID
	sig, err := hsm.RoleSign(context.TODO(), hsm.localID, hs.CalcHash(msg.Data.Hash().Bytes(), hsm.curView.phase), types.SigBLS)
	if err != nil {
		return err
	}
	msg.Sig = sig
	msg.Quorum = types.NewMultiSignature(types.SigBLS)
	msg.Type = hs.MsgVoteCommit

	hsm.PublishHsMsg(hsm.ctx, msg)

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
		return xerrors.Errorf("phase state wrong")
	}

	err = hsm.curView.commitQuorum.Add(msg.From, msg.Sig)
	if err != nil {
		return err
	}

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

	// todo: send hm message out
	hsm.PublishHsMsg(hsm.ctx, hm)

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

	// leader publish out, replica need?
	hsm.INetService.PublishTxBlock(hsm.ctx, sb)

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
		return xerrors.Errorf("phase state wrong")
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
