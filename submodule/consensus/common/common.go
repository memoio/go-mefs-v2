package common

import (
	"time"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/types"
)

// DecisionMaker decides 1. proposal 2. whether accepts a proposal
// and applies proposal after view done
type DecisionMaker interface {
	// propose a proposal
	Propose() []byte

	// validate proposal
	OnPropose([]byte) error

	// apply proposal after consensus view done
	Apply([]byte) error
}

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

type ProposalChain interface {
	// retrieves a block given its hash. If it is not available locally, it will try to fetch the block
	Get(types.MsgID) (*Proposal, bool)

	// stores a block in the blockchain
	Put(p *Proposal)

	// Extends checks if the given block extends the branch of the target hash
	Extends(block, target *Proposal) bool

	// CurrentBlock return latest on-chain proposal
	CurrentBlock() *Proposal
}

type CommitteeManager interface {
	// get leader
	GetLeader(view uint64) uint64

	// quorum size
	GetQuorumSize() int

	// the number of members in the committee
	Size() int
}

type HotStuffApplication interface {
	api.IRole
	api.INetService

	CommitteeManager

	DecisionMaker

	// pacemaker / proposal chain
	CheckView(lastProposal []byte) error
	CurrentState() (uint64, string, uint64)
	View() uint64
	GetViewN([]byte) uint64
	GetHeight([]byte) uint64
	InitQC() (uint64, []byte, []byte, []byte)

	// synchronizer
	Synchronize([]byte) error
}
