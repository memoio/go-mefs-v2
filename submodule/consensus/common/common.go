package common

import (
	"time"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
)

type PaceMaker interface {
	// return current view
	View() uint64

	// intriger timeout
	TimeoutChannel() <-chan time.Time

	// notify that view has been changed and step into consensus
	ViewChanged()

	// advance to new view and notify timeout controller
	OnTimeout() bool

	// Consensus finish before timeout, notify pacemaker and stop timer
	DoneB4Timeout()
}

// DecisionMaker decides 1. proposal 2. whether accepts a proposal
// and applies proposal after view done
type DecisionMaker interface {
	// propose a proposal
	Propose() tx.MsgSet

	// validate proposal
	OnPropose(tx.RawBlock) error

	// apply proposal after consensus view done
	Apply(tx.SignedBlock) error
}

type CommitteeManager interface {
	GetMembers() []uint64
	// get leader
	GetLeader(view uint64) uint64

	// quorum size
	GetQuorumSize() int
}

type State interface {
	GetSyncStatus() bool
	GetHeight() uint64
	GetSlot() uint64
	GetBlockRoot(uint64) types.MsgID
}

type HotStuffApplication interface {
	api.IRole
	api.INetService

	CommitteeManager

	DecisionMaker

	State
}
