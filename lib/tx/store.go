package tx

import (
	"context"
	"math"

	lru "github.com/hashicorp/golang-lru"

	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

type Store interface {
	GetTxMsg(mid types.MsgID) (*SignedMessage, error)
	PutTxMsg(sm *SignedMessage, persist bool) error
	HasTxMsg(mid types.MsgID) (bool, error)

	GetTxMsgState(mid types.MsgID) (*MsgState, error)
	PutTxMsgState(mid types.MsgID, ms *MsgState) error

	GetTxBlock(bid types.MsgID) (*SignedBlock, error)
	PutTxBlock(tb *SignedBlock) (int, error)
	DeleteTxBlock(bid types.MsgID) error
	HasTxBlock(bid types.MsgID) (bool, error)

	GetTxBlockByHeight(ht uint64) (types.MsgID, error)
	DeleteTxBlockHeight(ht uint64) error
	PutTxBlockHeight(uint64, types.MsgID) error
}

var _ Store = (*TxStoreImpl)(nil)

type TxStoreImpl struct {
	ctx context.Context
	ds  store.KVStore

	msgCache *lru.ARCCache
	blkCache *lru.TwoQueueCache

	htCache *lru.ARCCache
}

func NewTxStore(ctx context.Context, ds store.KVStore) (*TxStoreImpl, error) {
	mc, err := lru.NewARC(1024)
	if err != nil {
		return nil, err
	}

	bc, err := lru.New2Q(1024)
	if err != nil {
		return nil, err
	}

	hc, err := lru.NewARC(1024)
	if err != nil {
		return nil, err
	}

	ts := &TxStoreImpl{
		ctx: ctx,
		ds:  ds,

		msgCache: mc,
		blkCache: bc,
		htCache:  hc,
	}

	return ts, nil
}

func (ts *TxStoreImpl) HasTxMsg(mid types.MsgID) (bool, error) {
	ok := ts.msgCache.Contains(mid)
	if ok {
		return ok, nil
	}

	key := store.NewKey(pb.MetaType_TX_MessageKey, mid.String())
	return ts.ds.Has(key)
}

func (ts *TxStoreImpl) GetTxMsg(mid types.MsgID) (*SignedMessage, error) {
	val, ok := ts.msgCache.Get(mid)
	if ok {
		return val.(*SignedMessage), nil
	}

	key := store.NewKey(pb.MetaType_TX_MessageKey, mid.String())

	res, err := ts.ds.Get(key)
	if err != nil {
		return nil, err
	}

	sm := new(SignedMessage)
	err = sm.Deserialize(res)
	if err != nil {
		return nil, err
	}

	ts.msgCache.Add(mid, sm)

	return sm, nil
}

func (ts *TxStoreImpl) PutTxMsg(sm *SignedMessage, persist bool) error {
	mid := sm.Hash()

	ok := ts.msgCache.Contains(mid)
	if ok {
		return nil
	}

	ts.msgCache.Add(mid, sm)

	if persist {
		key := store.NewKey(pb.MetaType_TX_MessageKey, mid.String())
		sbyte, err := sm.Serialize()
		if err != nil {
			return err
		}

		return ts.ds.Put(key, sbyte)
	}
	return nil
}

func (ts *TxStoreImpl) HasTxBlock(bid types.MsgID) (bool, error) {
	ok := ts.blkCache.Contains(bid)
	if ok {
		return ok, nil
	}
	key := store.NewKey(pb.MetaType_TX_BlockKey, bid.String())

	return ts.ds.Has(key)
}

func (ts *TxStoreImpl) DeleteTxBlock(bid types.MsgID) error {
	ok := ts.blkCache.Contains(bid)
	if ok {
		ts.blkCache.Remove(bid)
	}
	key := store.NewKey(pb.MetaType_TX_BlockKey, bid.String())

	return ts.ds.Delete(key)
}

func (ts *TxStoreImpl) GetTxBlock(bid types.MsgID) (*SignedBlock, error) {
	val, ok := ts.blkCache.Get(bid)
	if ok {
		return val.(*SignedBlock), nil
	}

	key := store.NewKey(pb.MetaType_TX_BlockKey, bid.String())

	res, err := ts.ds.Get(key)
	if err != nil {
		return nil, err
	}

	tb := new(SignedBlock)
	err = tb.Deserialize(res)
	if err != nil {
		return nil, err
	}

	ts.blkCache.Add(bid, tb)

	return tb, nil
}

func (ts *TxStoreImpl) PutTxBlock(tb *SignedBlock) (int, error) {
	bid := tb.Hash()

	has := ts.blkCache.Contains(bid)
	if has {
		return 0, nil
	}

	ts.blkCache.Add(bid, tb)
	key := store.NewKey(pb.MetaType_TX_BlockKey, bid.String())
	sbyte, err := tb.Serialize()
	if err != nil {
		return 0, err
	}

	err = ts.ds.Put(key, sbyte)
	if err != nil {
		return 0, err
	}

	err = ts.PutTxBlockHeight(tb.Height, bid)
	if err != nil {
		return 0, err
	}

	err = ts.PutTxBlockHeight(tb.Height-1, tb.PrevID)
	if err != nil {
		return 0, err
	}

	/*
		for _, mes := range tb.Msgs {
			ts.PutTxMsg(&mes)
		}
	*/

	return len(sbyte), nil
}

func (ts *TxStoreImpl) GetTxBlockByHeight(ht uint64) (types.MsgID, error) {
	bid := types.MsgID{}
	val, ok := ts.htCache.Get(ht)
	if ok {
		return val.(types.MsgID), nil
	}

	key := store.NewKey(pb.MetaType_Tx_BlockHeightKey, ht)
	res, err := ts.ds.Get(key)
	if err != nil {
		return bid, err
	}

	bid, err = types.FromBytes(res)
	if err != nil {
		return bid, err
	}

	ts.htCache.Add(ht, bid)

	return bid, nil
}

func (ts *TxStoreImpl) DeleteTxBlockHeight(ht uint64) error {
	ok := ts.htCache.Contains(ht)
	if ok {
		ts.htCache.Remove(ht)
	}

	key := store.NewKey(pb.MetaType_Tx_BlockHeightKey, ht)
	return ts.ds.Delete(key)
}

func (ts *TxStoreImpl) PutTxBlockHeight(ht uint64, bid types.MsgID) error {
	key := store.NewKey(pb.MetaType_Tx_BlockHeightKey, ht)

	err := ts.ds.Put(key, bid.Bytes())
	if err != nil {
		return err
	}

	ts.htCache.Add(ht, bid)

	return nil
}

func (ts *TxStoreImpl) PutTxMsgState(mid types.MsgID, ms *MsgState) error {
	key := store.NewKey(pb.MetaType_Tx_MessageStateKey, mid.String())
	msb, err := ms.Serialize()
	if err != nil {
		return err
	}

	return ts.ds.Put(key, msb)
}

func (ts *TxStoreImpl) GetTxMsgState(mid types.MsgID) (*MsgState, error) {
	key := store.NewKey(pb.MetaType_Tx_MessageStateKey, mid.String())
	val, err := ts.ds.Get(key)
	if err != nil {
		return nil, err
	}

	tms := new(MsgState)
	err = tms.Deserialize(val)
	if err != nil {
		return nil, err
	}

	// iter to find
	if tms.Height == math.MaxUint64 {
		return ts.GetTxMsgState(tms.BlockID)
	}

	return tms, err
}
