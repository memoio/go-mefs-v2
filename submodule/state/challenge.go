package state

import (
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
)

func (s *StateMgr) AddChallenge(msg *tx.Message) (types.MsgID, error) {
	s.Lock()
	defer s.Unlock()

	return s.root, nil
}
