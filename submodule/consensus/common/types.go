package common

import (
	"encoding/binary"

	"github.com/fxamacker/cbor/v2"

	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/types"
)

var logger = logging.Logger("consensus")

type MsgType uint16

const (
	MsgNewView MsgType = iota
	MsgPrepare
	MsgVotePrepare
	MsgPreCommit
	MsgVotePreCommit
	MsgCommit
	MsgVoteCommit
	MsgDecide
)

type PhaseState uint16

const (
	PhaseNew PhaseState = iota
	PhasePrepare
	PhasePreCommit
	PhaseCommit
	PhaseDecide
	PhaseFinal
)

// Proposal
type Header struct {
	Parent types.MsgID
	Leader uint64
	View   uint64 // view number
	Height uint64 // increase 1
}

func (h *Header) Hash() types.MsgID {
	buf, err := cbor.Marshal(h)
	if err != nil {
		return types.MsgIDUndef
	}

	return types.NewMsgID(buf)
}

func (h *Header) Serialize() ([]byte, error) {
	return cbor.Marshal(h)
}

func (h *Header) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, h)
}

type Proposal struct {
	Header
	Content []byte
}

func (p *Proposal) Hash() types.MsgID {
	buf, err := cbor.Marshal(p)
	if err != nil {
		return types.MsgIDUndef
	}

	return types.NewMsgID(buf)
}

func (p *Proposal) Serialize() ([]byte, error) {
	return cbor.Marshal(p)
}

func (p *Proposal) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, p)
}

func CalcHash(p Proposal, phase PhaseState) types.MsgID {
	h := p.Hash()
	buf := make([]byte, 2+len(h.Bytes()))
	binary.BigEndian.PutUint16(buf[:2], uint16(phase))
	copy(buf[2:], h.Bytes())

	return types.NewMsgID(buf)
}

type SignedProposal struct {
	Proposal
	Sig types.Signature
}

func (sp *SignedProposal) Serialize() ([]byte, error) {
	return cbor.Marshal(sp)
}

func (sp *SignedProposal) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, sp)
}

// MsgNewView, data: SignedProposal (PhaseNew and nil content)
// MsgPrepare, data: SignedProposal (PhasePrepare content), PhaseNew multi-signatures
// MsgVotePrepare, data: Signedproposal (PhasePrepare)
// MsgPreCommit, data: Signedproposal (PhasePreCommit), PhasePrepare multi-signatures
// MsgVotePreCommit, data: Signedproposal (PhasePreCommit)
// MsgCommit, data: Signedproposal (PhaseCommit), PhasePreCommit multi-signatures
// MsgVoteCommit, data: Signedproposal (PhaseCommit)
// MsgDecide, data: Signedproposal (PhaseDecide), PhaseCommit multi-signatures

// MsgStartNewView // for handling new view from app
// MsgTryPropose
// MsgSyncQC // for recovering liveness
type HotstuffMessage struct {
	From   uint64
	Type   MsgType
	Data   SignedProposal
	Quorum types.MultiSignature // quorum of previous phase, leader send
}

func (h *HotstuffMessage) Serialize() ([]byte, error) {
	return cbor.Marshal(h)
}

func (h *HotstuffMessage) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, h)
}
