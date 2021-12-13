package hotstuff

import (
	"encoding/binary"

	"github.com/fxamacker/cbor/v2"

	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
)

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

// MsgNewView, data: RawBlock (PhaseNew and nil content)
// MsgPrepare, data: RawBlock (PhasePrepare content), PhaseNew multi-signatures
// MsgVotePrepare, data: RawBlock (PhasePrepare)
// MsgPreCommit, data: RawBlock (PhasePreCommit), PhasePrepare multi-signatures
// MsgVotePreCommit, data: RawBlock (PhasePreCommit)
// MsgCommit, data: RawBlock (PhaseCommit), PhasePreCommit multi-signatures
// MsgVoteCommit, data: RawBlock (PhaseCommit)
// MsgDecide, data: RawBlock (PhaseDecide), PhaseCommit multi-signatures

// MsgStartNewView // for handling new view from app
// MsgTryPropose
// MsgSyncQC // for recovering liveness
type HotstuffMessage struct {
	From   uint64
	Type   MsgType
	Data   tx.RawBlock
	Sig    types.Signature      // sign(data+phase)
	Quorum types.MultiSignature // quorum of previous phase, leader send
}

func (h *HotstuffMessage) Serialize() ([]byte, error) {
	return cbor.Marshal(h)
}

func (h *HotstuffMessage) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, h)
}

func CalcHash(p tx.RawBlock, phase PhaseState) types.MsgID {
	h := p.Hash()
	buf := make([]byte, 2+len(h.Bytes()))
	binary.BigEndian.PutUint16(buf[:2], uint16(phase))
	copy(buf[2:], h.Bytes())

	return types.NewMsgID(buf)
}
