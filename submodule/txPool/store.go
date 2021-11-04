package txPool

import (
	"bytes"
	"context"
	"strconv"

	lru "github.com/hashicorp/golang-lru"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

var (
	TxMesKey    = []byte(strconv.Itoa(int(pb.MetaType_TX_MessageKey)))
	TxBlockKey  = []byte(strconv.Itoa(int(pb.MetaType_TX_BlockKey)))
	TxHeightKey = []byte(strconv.Itoa(int(pb.MetaType_Tx_HeightKey)))
)

type TxStore struct {
	ctx context.Context
	ds  store.KVStore

	msgCache *lru.ARCCache
	blkCache *lru.TwoQueueCache

	htCache *lru.ARCCache
}

func NewTxStore(ctx context.Context, ds store.KVStore) (*TxStore, error) {

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

	ts := &TxStore{
		ctx: ctx,
		ds:  ds,

		msgCache: mc,
		blkCache: bc,
		htCache:  hc,
	}

	return ts, nil
}

func (ts *TxStore) GetTXMsg(mid types.MsgID) (*tx.SignedMessage, error) {
	val, ok := ts.msgCache.Get(mid)
	if ok {
		return val.(*tx.SignedMessage), nil
	}

	key := concat(TxMesKey, mid.Bytes())

	res, err := ts.ds.Get(key)
	if err != nil {
		return nil, err
	}

	sm := new(tx.SignedMessage)
	err = sm.Deserilize(res)
	if err != nil {
		return nil, err
	}

	ts.msgCache.Add(mid, sm)

	return sm, nil
}

func (ts *TxStore) PutTXMsg(sm *tx.SignedMessage) error {
	mid, err := sm.Hash()
	if err != nil {
		return err
	}

	ok := ts.msgCache.Contains(mid)
	if ok {
		return nil
	}

	key := concat(TxMesKey, mid.Bytes())
	sbyte, err := sm.Serialize()
	if err != nil {
		return err
	}

	ts.msgCache.Add(mid, sm)

	return ts.ds.Put(key, sbyte)
}

func (ts *TxStore) GetTxBlock(types.MsgID) (*tx.Block, error) {
	return nil, nil
}

func (ts *TxStore) PutTxBlock(tb *tx.Block) error {
	return nil
}

func (ts *TxStore) GetTxBlockByHeight(ht uint64) (*tx.Block, error) {
	return nil, nil
}

func concat(vs ...[]byte) []byte {
	var b bytes.Buffer
	for _, v := range vs {
		b.Write(v)
	}
	return b.Bytes()
}
