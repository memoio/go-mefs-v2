package common

import (
	"github.com/memoio/go-mefs-v2/lib/tx"
)

type ConsensusMgr interface {
	MineBlock()
}

type PaceMaker interface {
	CreateBlockHeader() (tx.RawHeader, error)
}

// DecisionMaker decides 1. proposal 2. whether accepts a proposal
// and applies proposal after view done
type DecisionMaker interface {
	// propose a proposal
	Propose(tx.RawHeader) (tx.MsgSet, error)

	// validate proposal
	OnPropose(*tx.SignedBlock) error

	// apply proposal after consensus view done
	OnViewDone(*tx.SignedBlock) error
}

type CommitteeManager interface {
	GetMembers() []uint64

	// get leader
	GetLeader(view uint64) uint64

	// quorum size
	GetQuorumSize() int
}

type ConsensusApp interface {
	CommitteeManager

	PaceMaker

	DecisionMaker
}
