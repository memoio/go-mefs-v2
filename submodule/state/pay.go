package state

import (
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"golang.org/x/xerrors"
)

func (s *StateMgr) addPay(msg *tx.Message) error {
	pip := new(tx.PostIncomeParams)
	err := pip.Deserialize(msg.Params)
	if err != nil {
		return err
	}

	if pip.Epoch != s.ceInfo.previous.Epoch {
		return xerrors.Errorf("add post income epoch, expected %d, got %d", s.ceInfo.previous.Epoch, pip.Epoch)
	}

	if len(pip.Pros) != len(pip.Sig) {
		return xerrors.Errorf("add post income paras, expected %d, got %d", len(pip.Pros), len(pip.Sig))
	}

	kri, ok := s.rInfo[msg.From]
	if !ok {
		kri = s.loadRole(msg.From)
		s.rInfo[msg.From] = kri
	}

	if kri.base.Type != pb.RoleInfo_Keeper {
		return xerrors.Errorf("add post income role type wrong, expected %d, got %d", pb.RoleInfo_Keeper, kri.base.Type)
	}

	for i, pid := range pip.Pros {
		key := store.NewKey(pb.MetaType_ST_SegPayKey, pip.UserID, pid, pip.Epoch)
		data, err := s.ds.Get(key)
		if err != nil {
			return err
		}

		spi := new(types.SignedPostIncome)
		err = spi.Deserialize(data)
		if err != nil {
			return err
		}
		err = verify(kri.base, spi.Hash(), pip.Sig[i])
		if err != nil {
			return err
		}
		err = spi.Sign.Add(msg.From, pip.Sig[i])
		if err != nil {
			return err
		}

		data, err = spi.Serialize()
		if err != nil {
			return err
		}
		s.ds.Put(key, data)
	}

	return nil
}

func (s *StateMgr) canAddPay(msg *tx.Message) error {
	pip := new(tx.PostIncomeParams)
	err := pip.Deserialize(msg.Params)
	if err != nil {
		return err
	}

	if pip.Epoch != s.validateCeInfo.previous.Epoch {
		return xerrors.Errorf("add post income epoch, expected %d, got %d", s.validateCeInfo.previous.Epoch, pip.Epoch)
	}

	if len(pip.Pros) != len(pip.Sig) {
		return xerrors.Errorf("add post income paras, expected %d, got %d", len(pip.Pros), len(pip.Sig))
	}

	kri, ok := s.validateRInfo[msg.From]
	if !ok {
		kri = s.loadRole(msg.From)
		s.validateRInfo[msg.From] = kri
	}

	if kri.base.Type != pb.RoleInfo_Keeper {
		return xerrors.Errorf("add post income role type wrong, expected %d, got %d", pb.RoleInfo_Keeper, kri.base.Type)
	}

	for i, pid := range pip.Pros {
		key := store.NewKey(pb.MetaType_ST_SegPayKey, pip.UserID, pid, pip.Epoch)
		data, err := s.ds.Get(key)
		if err != nil {
			return err
		}

		spi := new(types.SignedPostIncome)
		err = spi.Deserialize(data)
		if err != nil {
			return err
		}
		err = verify(kri.base, spi.Hash(), pip.Sig[i])
		if err != nil {
			return err
		}
		err = spi.Sign.Add(msg.From, pip.Sig[i])
		if err != nil {
			return err
		}
	}

	return nil
}
