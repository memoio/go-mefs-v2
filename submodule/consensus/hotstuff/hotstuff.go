package hotstuff

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	bcommon "github.com/memoio/go-mefs-v2/submodule/consensus/common"
	"golang.org/x/xerrors"
)

// timeout will be moved to pacemaker, but clearview etc. will be opened for pacemaker
type HotstuffProtocolManager struct {
	localID uint64
	ds      store.KVStore

	// record
	lastProposal []byte // last applied
	lockedQC     types.MultiSignature
	prepareQC    types.MultiSignature

	views map[types.MsgID]*view

	curView types.MsgID

	// process
	app bcommon.HotStuffApplication
}

func NewHotstuffProtocolManager(a bcommon.HotStuffApplication, localID uint64) *HotstuffProtocolManager {
	manager := &HotstuffProtocolManager{
		app:   a,
		views: make(map[types.MsgID]*view),
	}

	return manager
}

func (hsm *HotstuffProtocolManager) handleMessage(msg *bcommon.HotstuffMessage) error {
	// verify data first
	ptype, ok := phaseMap[msg.Type]
	if ok {
		ok, err := hsm.app.RoleVerify(context.TODO(), msg.From, bcommon.CalcHash(msg.Data.Proposal, ptype).Bytes(), msg.Data.Sig)
		if err != nil || !ok {
			return err
		}
	}

	ptype, ok = preMap[msg.Type]
	if ok {
		proposal := bcommon.Proposal{
			Header:  msg.Data.Header,
			Content: msg.Data.Content,
		}
		switch ptype {
		// content is nil at phase new
		case bcommon.PhaseNew:
			proposal.Content = nil
		}

		ok, err := hsm.app.RoleVerifyMulti(context.TODO(), bcommon.CalcHash(proposal, ptype).Bytes(), msg.Quorum)
		if err != nil || !ok {
			return err
		}
	}

	switch msg.Type {
	case bcommon.MsgNewView:
		return hsm.handleNewViewVoteMsg(msg)

	case bcommon.MsgPrepare:
		return hsm.handlePrepareMsg(msg)
	case bcommon.MsgVotePrepare:
		return hsm.handlePrepareVoteMsg(msg)

	case bcommon.MsgPreCommit:
		return hsm.handlePreCommitMsg(msg)
	case bcommon.MsgVotePreCommit:
		return hsm.handlePreCommitVoteMsg(msg)

	case bcommon.MsgCommit:
		return hsm.handleCommitMsg(msg)
	case bcommon.MsgVoteCommit:
		return hsm.handleCommitVoteMsg(msg)

	case bcommon.MsgDecide:
		return hsm.handleDecideMsg(msg)

	default:
		fmt.Println("unknown hotstuff message type", msg.Type)
		return xerrors.Errorf("unknown hotstuff message type %d", msg.Type)
	}
}

var phaseMap map[bcommon.MsgType]bcommon.PhaseState
var preMap map[bcommon.MsgType]bcommon.PhaseState

func init() {
	phaseMap = make(map[bcommon.MsgType]bcommon.PhaseState)
	preMap = make(map[bcommon.MsgType]bcommon.PhaseState)

	phaseMap[bcommon.MsgNewView] = bcommon.PhaseNew
	phaseMap[bcommon.MsgPrepare] = bcommon.PhasePrepare
	phaseMap[bcommon.MsgPreCommit] = bcommon.PhasePreCommit
	phaseMap[bcommon.MsgCommit] = bcommon.PhaseCommit

	phaseMap[bcommon.MsgVotePrepare] = bcommon.PhasePrepare
	phaseMap[bcommon.MsgVotePreCommit] = bcommon.PhasePreCommit
	phaseMap[bcommon.MsgVoteCommit] = bcommon.PhaseCommit

	// validate its pre qc
	preMap[bcommon.MsgPrepare] = bcommon.PhaseNew
	preMap[bcommon.MsgPreCommit] = bcommon.PhasePrepare
	preMap[bcommon.MsgCommit] = bcommon.PhasePreCommit
	preMap[bcommon.MsgDecide] = bcommon.PhaseCommit
}

type view struct {
	header  bcommon.Header
	content []byte

	phase bcommon.PhaseState

	// only leader use this field
	highQuorum      types.MultiSignature // hash(MsgNewView, Header)
	prepareQuorum   types.MultiSignature // hash(MsgPrepare, Header, Cmd)
	preCommitQuorum types.MultiSignature // hash(MsgPreCommit, Proposer Header, Cmd)
	commitQuorum    types.MultiSignature // hash(MsgCommit, Proposer Header, Cmd)

	createdAt time.Time
}

func (hsm *HotstuffProtocolManager) newView() *view {
	// get current view number, block height, leader
	header := bcommon.Header{
		// base on parent msgID,
	}

	v, ok := hsm.views[header.Hash()]
	if ok {
		// redo ?
		return v
	}

	v = &view{
		header:    header,
		phase:     bcommon.PhasePrepare,
		createdAt: time.Now(),

		highQuorum:      types.NewMultiSignature(types.SigBLS),
		prepareQuorum:   types.NewMultiSignature(types.SigBLS),
		preCommitQuorum: types.NewMultiSignature(types.SigBLS),
		commitQuorum:    types.NewMultiSignature(types.SigBLS),
	}

	log.Println("create new view", "leader", v.header.Leader, "viewID", v.header.Hash())

	return v
}

// when time is up, enter into new view
// replicas send to leader
func (hsm *HotstuffProtocolManager) NewView() error {

	v := hsm.newView()

	if v.header.Leader == hsm.localID {
		return nil
	}

	v.phase = bcommon.PhaseNew

	// replica send out to leader
	sp := bcommon.SignedProposal{
		Proposal: bcommon.Proposal{
			Header: v.header,
		},
	}

	sig, err := hsm.app.RoleSign(context.TODO(), hsm.localID, bcommon.CalcHash(sp.Proposal, v.phase).Bytes(), types.SigBLS)
	if err != nil {
		return err
	}

	sp.Sig = sig

	hm := &bcommon.HotstuffMessage{
		From: hsm.localID,
		Type: bcommon.MsgNewView,
		Data: sp,
	}

	err = v.highQuorum.Add(hsm.localID, sig)
	if err != nil {
		return err
	}

	// todo: send hm message out
	hm.Type = bcommon.MsgNewView

	return nil
}

// leader handle new view msg from replicas
func (hsm *HotstuffProtocolManager) handleNewViewVoteMsg(msg *bcommon.HotstuffMessage) error {
	log.Println("handleNewViewMsg got new view message", "from", msg.From, "viewId", msg.Data.Proposal.Header.Hash())

	// not handle it
	if hsm.localID != msg.Data.Leader {
		return nil
	}

	// todo: fast sanity check on datalast (avoid to create useless view)

	v, exist := hsm.views[msg.Data.Proposal.Header.Hash()]
	if !exist {
		v = hsm.newView()
		if !v.header.Hash().Equal(msg.Data.Proposal.Header.Hash()) {
			return xerrors.Errorf("view not match")
		}
		hsm.views[v.header.Hash()] = v
	}

	v.phase = bcommon.PhaseNew

	err := v.highQuorum.Add(msg.From, msg.Data.Sig)
	if err != nil {
		return err
	}

	if v.highQuorum.Len() >= hsm.app.GetQuorumSize() {
		hsm.curView = v.header.Hash()
		return hsm.tryPropose()
	}

	return nil
}

// lead send prepare msg
func (hsm *HotstuffProtocolManager) tryPropose() error {
	v, ok := hsm.views[hsm.curView]
	if !ok {
		return xerrors.Errorf("no view")
	}

	if v.phase != bcommon.PhaseNew {
		return xerrors.Errorf("phase state error")
	}

	v.phase = bcommon.PhasePrepare

	// ensure previous proposals have been executed

	proposal := hsm.app.Propose()

	v.content = proposal

	sp := bcommon.SignedProposal{
		Proposal: bcommon.Proposal{
			Header:  v.header,
			Content: v.content,
		},
	}

	sig, err := hsm.app.RoleSign(context.TODO(), hsm.localID, bcommon.CalcHash(sp.Proposal, v.phase).Bytes(), types.SigBLS)
	if err != nil {
		return err
	}

	sp.Sig = sig

	hm := &bcommon.HotstuffMessage{
		From:   hsm.localID,
		Type:   bcommon.MsgPrepare,
		Data:   sp,
		Quorum: v.highQuorum,
	}

	// todo: send hm message out
	hm.Type = bcommon.MsgPrepare

	return nil
}

// replica response prepare msg
func (hsm *HotstuffProtocolManager) handlePrepareMsg(msg *bcommon.HotstuffMessage) error {
	v, exist := hsm.views[msg.Data.Proposal.Header.Hash()]
	if !exist {
		v = hsm.newView()
		if !v.header.Hash().Equal(msg.Data.Proposal.Header.Hash()) {
			return xerrors.Errorf("view not match")
		}
		hsm.views[v.header.Hash()] = v
	}

	v.phase = bcommon.PhasePrepare

	v.highQuorum = msg.Quorum

	// validate propose
	err := hsm.app.OnPropose(msg.Data.Content)
	if err != nil {
		return err
	}

	v.phase = bcommon.PhasePrepare
	v.content = msg.Data.Content

	msg.From = hsm.localID
	sig, err := hsm.app.RoleSign(context.TODO(), hsm.localID, bcommon.CalcHash(msg.Data.Proposal, v.phase).Bytes(), types.SigBLS)
	if err != nil {
		return err
	}
	msg.Data.Sig = sig
	msg.Quorum = types.NewMultiSignature(types.SigBLS)
	msg.Type = bcommon.MsgVotePrepare

	// send to leader

	return nil
}

// leader handle prepare response
// leader send precommit
func (hsm *HotstuffProtocolManager) handlePrepareVoteMsg(msg *bcommon.HotstuffMessage) error {
	v, exist := hsm.views[msg.Data.Proposal.Header.Hash()]
	if !exist {
		v = hsm.newView()
		if !v.header.Hash().Equal(msg.Data.Proposal.Header.Hash()) {
			return xerrors.Errorf("view not match")
		}
		hsm.views[v.header.Hash()] = v
	}

	err := v.prepareQuorum.Add(msg.From, msg.Data.Sig)
	if err != nil {
		return err
	}

	if v.prepareQuorum.Len() >= hsm.app.GetQuorumSize() {
		hsm.prepareQC = v.prepareQuorum
		return hsm.tryPreCommit()
	}

	return nil
}

// leader send precommit message
func (hsm *HotstuffProtocolManager) tryPreCommit() error {
	v, ok := hsm.views[hsm.curView]
	if !ok {
		return xerrors.Errorf("no view")
	}

	if v.phase != bcommon.PhasePrepare {
		return xerrors.Errorf("phase state error")
	}

	v.phase = bcommon.PhasePreCommit

	sp := bcommon.SignedProposal{
		Proposal: bcommon.Proposal{
			Header:  v.header,
			Content: v.content,
		},
	}

	sig, err := hsm.app.RoleSign(context.TODO(), hsm.localID, bcommon.CalcHash(sp.Proposal, v.phase).Bytes(), types.SigBLS)
	if err != nil {
		return err
	}

	sp.Sig = sig

	hm := &bcommon.HotstuffMessage{
		From:   hsm.localID,
		Type:   bcommon.MsgPreCommit,
		Data:   sp,
		Quorum: v.prepareQuorum,
	}

	// todo: send hm message out
	hm.Type = bcommon.MsgPreCommit

	return nil
}

// replica handle precommit message
func (hsm *HotstuffProtocolManager) handlePreCommitMsg(msg *bcommon.HotstuffMessage) error {
	v, exist := hsm.views[msg.Data.Proposal.Header.Hash()]
	if !exist {
		v = hsm.newView()
		if !v.header.Hash().Equal(msg.Data.Proposal.Header.Hash()) {
			return xerrors.Errorf("view not match")
		}
		hsm.views[v.header.Hash()] = v
	}

	v.prepareQuorum = msg.Quorum

	v.phase = bcommon.PhasePreCommit

	msg.From = hsm.localID
	sig, err := hsm.app.RoleSign(context.TODO(), hsm.localID, bcommon.CalcHash(msg.Data.Proposal, v.phase).Bytes(), types.SigBLS)
	if err != nil {
		return err
	}
	msg.Data.Sig = sig
	msg.Quorum = types.NewMultiSignature(types.SigBLS)
	msg.Type = bcommon.MsgVotePreCommit

	// send to leader

	return nil
}

// leader handle precommit response
func (hsm *HotstuffProtocolManager) handlePreCommitVoteMsg(msg *bcommon.HotstuffMessage) error {
	v, exist := hsm.views[msg.Data.Proposal.Header.Hash()]
	if !exist {
		v = hsm.newView()
		if !v.header.Hash().Equal(msg.Data.Proposal.Header.Hash()) {
			return xerrors.Errorf("view not match")
		}
		hsm.views[v.header.Hash()] = v
	}

	err := v.preCommitQuorum.Add(msg.From, msg.Data.Sig)
	if err != nil {
		return err
	}

	if v.preCommitQuorum.Len() >= hsm.app.GetQuorumSize() {
		return hsm.tryCommit()
	}
	return nil
}

// leader send commit
func (hsm *HotstuffProtocolManager) tryCommit() error {
	v, ok := hsm.views[hsm.curView]
	if !ok {
		return xerrors.Errorf("no view")
	}

	if v.phase != bcommon.PhasePreCommit {
		return xerrors.Errorf("phase state error")
	}

	v.phase = bcommon.PhaseCommit

	sp := bcommon.SignedProposal{
		Proposal: bcommon.Proposal{
			Header:  v.header,
			Content: v.content,
		},
	}

	sig, err := hsm.app.RoleSign(context.TODO(), hsm.localID, bcommon.CalcHash(sp.Proposal, v.phase).Bytes(), types.SigBLS)
	if err != nil {
		return err
	}

	sp.Sig = sig

	hm := &bcommon.HotstuffMessage{
		From:   hsm.localID,
		Type:   bcommon.MsgCommit,
		Data:   sp,
		Quorum: v.preCommitQuorum,
	}

	// todo: send hm message out
	hm.Type = bcommon.MsgCommit

	return nil
}

// replica handle commit message
func (hsm *HotstuffProtocolManager) handleCommitMsg(msg *bcommon.HotstuffMessage) error {
	v, exist := hsm.views[msg.Data.Proposal.Header.Hash()]
	if !exist {
		v = hsm.newView()
		if !v.header.Hash().Equal(msg.Data.Proposal.Header.Hash()) {
			return xerrors.Errorf("view not match")
		}
		hsm.views[v.header.Hash()] = v
	}
	v.phase = bcommon.PhaseCommit

	v.prepareQuorum = msg.Quorum

	msg.From = hsm.localID
	sig, err := hsm.app.RoleSign(context.TODO(), hsm.localID, bcommon.CalcHash(msg.Data.Proposal, v.phase).Bytes(), types.SigBLS)
	if err != nil {
		return err
	}
	msg.Data.Sig = sig
	msg.Quorum = types.NewMultiSignature(types.SigBLS)
	msg.Type = bcommon.MsgVoteCommit

	return nil
}

// leader handle commit response
func (hsm *HotstuffProtocolManager) handleCommitVoteMsg(msg *bcommon.HotstuffMessage) error {
	v, exist := hsm.views[msg.Data.Proposal.Header.Hash()]
	if !exist {
		v = hsm.newView()
		if !v.header.Hash().Equal(msg.Data.Proposal.Header.Hash()) {
			return xerrors.Errorf("view not match")
		}
		hsm.views[v.header.Hash()] = v
	}

	err := v.commitQuorum.Add(msg.From, msg.Data.Sig)
	if err != nil {
		return err
	}

	if v.commitQuorum.Len() >= hsm.app.GetQuorumSize() {
		hsm.lockedQC = v.commitQuorum
		return hsm.tryDecide()
	}
	return nil
}

// leader sends decide message
func (hsm *HotstuffProtocolManager) tryDecide() error {
	v, ok := hsm.views[hsm.curView]
	if !ok {
		return xerrors.Errorf("no view")
	}

	v.phase = bcommon.PhaseDecide

	sp := bcommon.SignedProposal{
		Proposal: bcommon.Proposal{
			Header:  v.header,
			Content: v.content,
		},
	}

	sig, err := hsm.app.RoleSign(context.TODO(), hsm.localID, bcommon.CalcHash(sp.Proposal, v.phase).Bytes(), types.SigBLS)
	if err != nil {
		return err
	}

	sp.Sig = sig

	hm := &bcommon.HotstuffMessage{
		From:   hsm.localID,
		Type:   bcommon.MsgDecide,
		Data:   sp,
		Quorum: v.commitQuorum,
	}

	// todo: send hm message out
	hm.Type = bcommon.MsgDecide

	// apply
	err = hsm.app.Apply(v.content)
	if err != nil {
		return err
	}

	v.phase = bcommon.PhaseFinal

	return nil
}

// replica handle decide message
func (hsm *HotstuffProtocolManager) handleDecideMsg(msg *bcommon.HotstuffMessage) error {
	v, exist := hsm.views[msg.Data.Proposal.Header.Hash()]
	if !exist {
		v = hsm.newView()
		if !v.header.Hash().Equal(msg.Data.Proposal.Header.Hash()) {
			return xerrors.Errorf("view not match")
		}
		hsm.views[v.header.Hash()] = v
	}

	v.phase = bcommon.PhaseDecide

	v.commitQuorum = msg.Quorum

	err := hsm.app.Apply(v.content)
	if err != nil {
		return err
	}

	v.phase = bcommon.PhaseFinal

	return nil
}
