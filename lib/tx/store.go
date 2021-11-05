package tx

import (
	"context"

	lru "github.com/hashicorp/golang-lru"

	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

type TxStore interface {
	GetTXMsg(mid types.MsgID) (*SignedMessage, error)
	PutTXMsg(sm *SignedMessage) error

	GetTxBlock(bid types.MsgID) (*Block, error)
	PutTxBlock(tb *Block) error

	GetTxBlockByHeight(ht uint64) (types.MsgID, error)
}

var _ TxStore = (*TxStoreImpl)(nil)

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

func (ts *TxStoreImpl) GetTXMsg(mid types.MsgID) (*SignedMessage, error) {
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

func (ts *TxStoreImpl) PutTXMsg(sm *SignedMessage) error {
	mid, err := sm.Hash()
	if err != nil {
		return err
	}

	ok := ts.msgCache.Contains(mid)
	if ok {
		return nil
	}

	key := store.NewKey(pb.MetaType_TX_MessageKey, mid.String())
	sbyte, err := sm.Serialize()
	if err != nil {
		return err
	}

	ts.msgCache.Add(mid, sm)

	return ts.ds.Put(key, sbyte)
}

func (ts *TxStoreImpl) GetTxBlock(bid types.MsgID) (*Block, error) {
	val, ok := ts.blkCache.Get(bid)
	if ok {
		return val.(*Block), nil
	}

	key := store.NewKey(pb.MetaType_TX_BlockKey, bid.String())

	res, err := ts.ds.Get(key)
	if err != nil {
		return nil, err
	}

	tb := new(Block)
	err = tb.Deserialize(res)
	if err != nil {
		return nil, err
	}

	ts.blkCache.Add(bid, tb)

	return tb, nil
}

func (ts *TxStoreImpl) PutTxBlock(tb *Block) error {
	bid, err := tb.Hash()
	if err != nil {
		return err
	}

	ok := ts.blkCache.Contains(bid)
	if ok {
		return nil
	}

	ts.blkCache.Add(bid, tb)
	ts.htCache.Add(tb.Height, bid)

	key := store.NewKey(pb.MetaType_TX_BlockKey, bid.String())
	sbyte, err := tb.Serialize()
	if err != nil {
		return err
	}

	err = ts.ds.Put(key, sbyte)
	if err != nil {
		return err
	}

	key = store.NewKey(pb.MetaType_Tx_HeightKey, tb.Height)

	err = ts.ds.Put(key, bid.Bytes())
	if err != nil {
		return err
	}

	// store msg state; for what?
	for i, mes := range tb.Txs {
		ms := &TxMsgState{
			BlockID: mes,
			Height:  tb.Height,
			Status:  tb.Receipts[i].Err,
		}

		msb, err := ms.Serialize()
		if err == nil {
			key := store.NewKey(pb.MetaType_Tx_MessageStateKey, mes.String())
			ts.ds.Put(key, msb)
		}
	}

	return nil
}

func (ts *TxStoreImpl) GetTxBlockByHeight(ht uint64) (types.MsgID, error) {
	bid := types.MsgID{}
	val, ok := ts.htCache.Get(ht)
	if ok {
		return val.(types.MsgID), nil
	}

	key := store.NewKey(pb.MetaType_Tx_HeightKey, ht)

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

func (ts *TxStoreImpl) GetTxMsgState(mid types.MsgID) (*TxMsgState, error) {
	key := store.NewKey(pb.MetaType_Tx_MessageStateKey, mid.String())
	val, err := ts.ds.Get(key)
	if err != nil {
		return nil, err
	}

	tms := new(TxMsgState)
	err = tms.Deserialize(val)
	return tms, err
}
