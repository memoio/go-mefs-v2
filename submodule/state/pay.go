package state

import (
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"golang.org/x/xerrors"
)

func (s *StateMgr) addPay(msg *tx.Message, tds store.TxnStore) error {
	pip := new(tx.PostIncomeParams)
	err := pip.Deserialize(msg.Params)
	if err != nil {
		return err
	}

	if pip.Epoch != s.ceInfo.previous.Epoch {
		return xerrors.Errorf("add post income epoch, expected %d, got %d", s.ceInfo.previous.Epoch, pip.Epoch)
	}

	// todo: verify proID

	kri, ok := s.rInfo[msg.From]
	if !ok {
		kri = s.loadRole(msg.From)
		s.rInfo[msg.From] = kri
	}

	if kri.base.Type != pb.RoleInfo_Keeper {
		return xerrors.Errorf("add post income role type wrong, expected %d, got %d", pb.RoleInfo_Keeper, kri.base.Type)
	}

	spi := new(types.AccPostIncome)
	key := store.NewKey(pb.MetaType_ST_SegPayKey, 0, pip.Income.ProID, pip.Epoch)
	data, err := tds.Get(key)
	if err != nil {
		return err
	}

	err = spi.Deserialize(data)
	if err != nil {
		return err
	}

	if spi.Value.Cmp(pip.Income.Value) != 0 {
		return xerrors.Errorf("income value not equal, expected %d, got %d", pip.Income.Value, spi.Value)
	}

	if spi.Penalty.Cmp(pip.Income.Penalty) != 0 {
		return xerrors.Errorf("income penalty not equal, expected %d, got %d", pip.Income.Penalty, spi.Penalty)
	}

	err = verify(kri.base, pip.Income.Hash(), pip.Sig)
	if err != nil {
		return err
	}

	sapi := types.SignedAccPostIncome{
		AccPostIncome: pip.Income,
		Sig:           types.NewMultiSignature(types.SigSecp256k1),
	}
	key = store.NewKey(pb.MetaType_ST_SegPayComfirmKey, pip.Income.ProID, pip.Epoch)
	data, err = tds.Get(key)
	if err == nil {
		sapi.Deserialize(data)
	}

	if sapi.ProID != spi.ProID {
		return xerrors.Errorf("proID is not right, expected %d, got %d", sapi.ProID, spi.ProID)
	}

	if sapi.Value.Cmp(spi.Value) != 0 {
		return xerrors.Errorf("income value is not right, expected %d, got %d", sapi.Value, spi.Value)
	}

	if sapi.Penalty.Cmp(spi.Penalty) != 0 {
		return xerrors.Errorf("income Penalty is not right, expected %d, got %d", sapi.Penalty, spi.Penalty)
	}

	err = sapi.Sig.Add(msg.From, pip.Sig)
	if err != nil {
		return err
	}

	data, err = sapi.Serialize()
	if err != nil {
		return err
	}
	err = tds.Put(key, data)
	if err != nil {
		return err
	}

	// save lastest
	key = store.NewKey(pb.MetaType_ST_SegPayComfirmKey, pip.Income.ProID)
	err = tds.Put(key, data)
	if err != nil {
		return err
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

	kri, ok := s.validateRInfo[msg.From]
	if !ok {
		kri = s.loadRole(msg.From)
		s.validateRInfo[msg.From] = kri
	}

	if kri.base.Type != pb.RoleInfo_Keeper {
		return xerrors.Errorf("add post income role type wrong, expected %d, got %d", pb.RoleInfo_Keeper, kri.base.Type)
	}

	spi := new(types.AccPostIncome)
	key := store.NewKey(pb.MetaType_ST_SegPayKey, 0, pip.Income.ProID, pip.Epoch)
	data, err := s.ds.Get(key)
	if err != nil {
		return err
	}

	err = spi.Deserialize(data)
	if err != nil {
		return err
	}

	if spi.Value.Cmp(pip.Income.Value) != 0 {
		return xerrors.Errorf("income value not equal, expected %d, got %d", pip.Income.Value, spi.Value)
	}

	if spi.Penalty.Cmp(pip.Income.Penalty) != 0 {
		return xerrors.Errorf("income penalty not equal, expected %d, got %d", pip.Income.Penalty, spi.Penalty)
	}

	err = verify(kri.base, pip.Income.Hash(), pip.Sig)
	if err != nil {
		return err
	}

	sapi := types.SignedAccPostIncome{
		AccPostIncome: pip.Income,
		Sig:           types.NewMultiSignature(types.SigSecp256k1),
	}
	key = store.NewKey(pb.MetaType_ST_SegPayComfirmKey, pip.Income.ProID, pip.Epoch)
	data, err = s.ds.Get(key)
	if err == nil {
		spi.Deserialize(data)
	}

	if sapi.ProID != spi.ProID {
		return xerrors.Errorf("proID is not right, expected %d, got %d", sapi.ProID, spi.ProID)
	}

	if sapi.Value.Cmp(spi.Value) != 0 {
		return xerrors.Errorf("income value is not right, expected %d, got %d", sapi.Value, spi.Value)
	}

	if sapi.Penalty.Cmp(spi.Penalty) != 0 {
		return xerrors.Errorf("income Penalty is not right, expected %d, got %d", sapi.Penalty, spi.Penalty)
	}

	err = sapi.Sig.Add(msg.From, pip.Sig)
	if err != nil {
		return err
	}

	return nil
}
