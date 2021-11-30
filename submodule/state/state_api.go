package state

import "github.com/memoio/go-mefs-v2/lib/types"

func (s *StateMgr) GetRoot() types.MsgID {
	return s.root
}

func (s *StateMgr) GetEpoch() uint64 {
	return s.epoch
}

func (s *StateMgr) GetHeight() uint64 {
	return s.height
}

func (s *StateMgr) GetChalEpoch() *ChalEpoch {
	s.RLock()
	defer s.RUnlock()

	return &ChalEpoch{
		Epoch:  s.epochInfo.Epoch,
		Height: s.epochInfo.Height,
		Seed:   s.epochInfo.Seed,
	}
}
