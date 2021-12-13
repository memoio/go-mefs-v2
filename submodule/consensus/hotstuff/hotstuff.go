package hotstuff

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/memoio/go-mefs-v2/build"
	hs "github.com/memoio/go-mefs-v2/lib/hotstuff"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	bcommon "github.com/memoio/go-mefs-v2/submodule/consensus/common"
	"golang.org/x/xerrors"
)

// timeout will be moved to pacemaker, but clearview etc. will be opened for pacemaker
type HotstuffProtocolManager struct {
	localID uint64

	curView *view

	// process
	app bcommon.HotStuffApplication
}

func NewHotstuffProtocolManager(localID uint64, a bcommon.HotStuffApplication) *HotstuffProtocolManager {
	manager := &HotstuffProtocolManager{
		localID: localID,
		app:     a,
		curView: &view{
			header: tx.RawHeader{
				Version: 1,
			},
		},
	}

	return manager
}

func (hsm *HotstuffProtocolManager) handleMessage(msg *hs.HotstuffMessage) error {
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

func (hsm *HotstuffProtocolManager) checkView(msg *hs.HotstuffMessage) error {
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
			h = hs.CalcHash(proposal, pType).Bytes()
		case hs.PhasePrepare:
			proposal.MsgSet = msg.Data.MsgSet
			h = hs.CalcHash(proposal, pType).Bytes()
		case hs.PhasePreCommit:
			proposal.MsgSet = hsm.curView.txs
			h = hs.CalcHash(proposal, pType).Bytes()
		case hs.PhaseCommit:
			proposal.MsgSet = hsm.curView.txs
			h = hs.CalcHash(proposal, pType).Bytes()
		}

		ok, err := hsm.app.RoleVerifyMulti(context.TODO(), h, msg.Quorum)
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
		ok, err := hsm.app.RoleVerify(context.TODO(), msg.From, hs.CalcHash(msg.Data, pType).Bytes(), msg.Sig)
		if err != nil {
			return err
		}
		if !ok {
			return xerrors.Errorf("sign is invalid")
		}
	}

	return xerrors.Errorf("empty curView")
}

func (hsm *HotstuffProtocolManager) newView() error {
	slot := uint64(time.Now().Unix()-build.BaseTime) / build.SlotDuration

	// handle last one;
	if hsm.curView.header.Slot < slot {
		if hsm.curView.phase != hs.PhaseFinal && hsm.curView.commitQuorum.Len() > hsm.app.GetQuorumSize() {
			sb := tx.SignedBlock{
				RawBlock: tx.RawBlock{
					RawHeader: hsm.curView.header,
					MsgSet:    hsm.curView.txs,
				},
				MultiSignature: hsm.curView.commitQuorum,
			}

			// apply last one
			err := hsm.app.Apply(sb)
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
	syned := hsm.app.GetSyncStatus()
	if !syned {
		return xerrors.Errorf("not sync")
	}

	ht := hsm.app.GetHeight()
	bid := hsm.app.GetBlockRoot(ht - 1)
	pslot := hsm.app.GetSlot()

	slot = uint64(hsm.curView.createdAt.Unix()-build.BaseTime) / build.SlotDuration
	if slot <= pslot {
		return xerrors.Errorf("slot time not up")
	}

	hsm.curView.header = tx.RawHeader{
		Version: 1,
		Slot:    uint64(hsm.curView.createdAt.Unix()-build.BaseTime) / build.SlotDuration,
		Height:  ht,
		MinerID: hsm.app.GetLeader(slot),
		PrevID:  bid,
	}

	log.Println("create new view", "leader", hsm.curView.header.MinerID, "view slot", hsm.curView.header.Slot)

	return nil
}

// when time is up, enter into new view
// replicas send to leader
func (hsm *HotstuffProtocolManager) NewView() error {
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

	sig, err := hsm.app.RoleSign(context.TODO(), hsm.localID, hs.CalcHash(sp, hsm.curView.phase).Bytes(), types.SigBLS)
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
	hm.Type = hs.MsgNewView

	return nil
}

// leader handle new view msg from replicas
func (hsm *HotstuffProtocolManager) handleNewViewVoteMsg(msg *hs.HotstuffMessage) error {
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
func (hsm *HotstuffProtocolManager) tryPropose() error {
	if hsm.curView.phase != hs.PhaseNew {
		return xerrors.Errorf("phase state error")
	}

	hsm.curView.phase = hs.PhasePrepare

	hsm.curView.txs = hsm.app.Propose()

	sp := tx.RawBlock{
		RawHeader: hsm.curView.header,
		MsgSet:    hsm.curView.txs,
	}

	sig, err := hsm.app.RoleSign(context.TODO(), hsm.localID, hs.CalcHash(sp, hsm.curView.phase).Bytes(), types.SigBLS)
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
	hm.Type = hs.MsgPrepare

	return nil
}

// replica response prepare msg
func (hsm *HotstuffProtocolManager) handlePrepareMsg(msg *hs.HotstuffMessage) error {
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
	err = hsm.app.OnPropose(msg.Data)
	if err != nil {
		return err
	}

	hsm.curView.phase = hs.PhasePrepare
	hsm.curView.highQuorum = msg.Quorum

	hsm.curView.txs = msg.Data.MsgSet

	msg.From = hsm.localID
	sig, err := hsm.app.RoleSign(context.TODO(), hsm.localID, hs.CalcHash(msg.Data, hsm.curView.phase).Bytes(), types.SigBLS)
	if err != nil {
		return err
	}
	msg.Sig = sig
	msg.Quorum = types.NewMultiSignature(types.SigBLS)
	msg.Type = hs.MsgVotePrepare

	// send to leader

	return nil
}

// leader handle prepare response
// leader send precommit
func (hsm *HotstuffProtocolManager) handlePrepareVoteMsg(msg *hs.HotstuffMessage) error {
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
func (hsm *HotstuffProtocolManager) tryPreCommit() error {
	if hsm.curView.phase != hs.PhasePrepare {
		return xerrors.Errorf("phase state error")
	}

	hsm.curView.phase = hs.PhasePreCommit

	sp := tx.RawBlock{
		RawHeader: hsm.curView.header,
		MsgSet:    hsm.curView.txs,
	}

	sig, err := hsm.app.RoleSign(context.TODO(), hsm.localID, hs.CalcHash(sp, hsm.curView.phase).Bytes(), types.SigBLS)
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
	hm.Type = hs.MsgPreCommit

	return nil
}

// replica handle precommit message
func (hsm *HotstuffProtocolManager) handlePreCommitMsg(msg *hs.HotstuffMessage) error {
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
	sig, err := hsm.app.RoleSign(context.TODO(), hsm.localID, hs.CalcHash(msg.Data, hsm.curView.phase).Bytes(), types.SigBLS)
	if err != nil {
		return err
	}
	msg.Sig = sig
	msg.Quorum = types.NewMultiSignature(types.SigBLS)
	msg.Type = hs.MsgVotePreCommit

	// send to leader

	return nil
}

// leader handle precommit response
func (hsm *HotstuffProtocolManager) handlePreCommitVoteMsg(msg *hs.HotstuffMessage) error {
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
func (hsm *HotstuffProtocolManager) tryCommit() error {
	if hsm.curView.phase != hs.PhasePreCommit {
		return xerrors.Errorf("phase state error")
	}

	hsm.curView.phase = hs.PhaseCommit

	sp := tx.RawBlock{
		RawHeader: hsm.curView.header,
		MsgSet:    hsm.curView.txs,
	}

	sig, err := hsm.app.RoleSign(context.TODO(), hsm.localID, hs.CalcHash(sp, hsm.curView.phase).Bytes(), types.SigBLS)
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
	hm.Type = hs.MsgCommit

	return nil
}

// replica handle commit message
func (hsm *HotstuffProtocolManager) handleCommitMsg(msg *hs.HotstuffMessage) error {
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
	sig, err := hsm.app.RoleSign(context.TODO(), hsm.localID, hs.CalcHash(msg.Data, hsm.curView.phase).Bytes(), types.SigBLS)
	if err != nil {
		return err
	}
	msg.Sig = sig
	msg.Quorum = types.NewMultiSignature(types.SigBLS)
	msg.Type = hs.MsgVoteCommit

	return nil
}

// leader handle commit response
func (hsm *HotstuffProtocolManager) handleCommitVoteMsg(msg *hs.HotstuffMessage) error {
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
func (hsm *HotstuffProtocolManager) tryDecide() error {
	if hsm.curView.phase != hs.PhaseCommit {
		return xerrors.Errorf("phase state error")
	}

	hsm.curView.phase = hs.PhaseDecide

	sp := tx.RawBlock{
		RawHeader: hsm.curView.header,
		MsgSet:    hsm.curView.txs,
	}

	sig, err := hsm.app.RoleSign(context.TODO(), hsm.localID, hs.CalcHash(sp, hsm.curView.phase).Bytes(), types.SigBLS)
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
	hm.Type = hs.MsgDecide

	// apply
	sb := tx.SignedBlock{
		RawBlock:       sp,
		MultiSignature: hsm.curView.commitQuorum,
	}

	err = hsm.app.Apply(sb)
	if err != nil {
		return err
	}

	hsm.curView.phase = hs.PhaseFinal

	return nil
}

// replica handle decide message
func (hsm *HotstuffProtocolManager) handleDecideMsg(msg *hs.HotstuffMessage) error {
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

	sb := tx.SignedBlock{
		RawBlock: tx.RawBlock{
			RawHeader: hsm.curView.header,
			MsgSet:    hsm.curView.txs,
		},
		MultiSignature: hsm.curView.commitQuorum,
	}

	err = hsm.app.Apply(sb)
	if err != nil {
		return err
	}

	hsm.curView.phase = hs.PhaseFinal

	return nil
}
