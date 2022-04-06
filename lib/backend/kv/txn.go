package kv

import (
	"github.com/dgraph-io/badger/v2"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/lib/types/store"
)

var _ store.TxnStore = (*txn)(nil)

type txn struct {
	db  *BadgerStore
	txn *badger.Txn
}

func (t *txn) GetNext(key []byte, bandwidth int) (uint64, error) {
	return t.db.GetNext(key, bandwidth)
}

func (t *txn) Put(key, value []byte) error {
	t.db.closeLk.RLock()
	defer t.db.closeLk.RUnlock()
	if t.db.closed {
		return ErrClosed
	}

	return t.txn.Set(key, value)
}

// key not found is not as error
func (t *txn) Get(key []byte) (value []byte, err error) {
	t.db.closeLk.RLock()
	defer t.db.closeLk.RUnlock()
	if t.db.closed {
		return nil, ErrClosed
	}

	item, err := t.txn.Get(key)
	if err != nil {
		return nil, xerrors.Errorf("get %s fail: %w", string(key), err)
	}

	return item.ValueCopy(nil)
}

func (t *txn) Has(key []byte) (bool, error) {
	t.db.closeLk.RLock()
	defer t.db.closeLk.RUnlock()
	if t.db.closed {
		return false, ErrClosed
	}

	_, err := t.txn.Get(key)
	if err != nil {
		return false, xerrors.Errorf("get %s fail: %w", string(key), err)
	}
	return true, nil
}

func (t *txn) Delete(key []byte) error {
	t.db.closeLk.RLock()
	defer t.db.closeLk.RUnlock()
	if t.db.closed {
		return ErrClosed
	}

	return t.txn.Delete(key)
}

func (t *txn) Commit() error {
	t.db.closeLk.RLock()
	defer t.db.closeLk.RUnlock()
	if t.db.closed {
		return ErrClosed
	}

	return t.txn.Commit()
}

func (t *txn) Size() store.DiskStats {
	return t.db.Size()
}

func (t *txn) Sync() error {
	return t.db.Sync()
}

func (t *txn) Close() error {
	return t.txn.Commit()
}

func (t *txn) Discard() {
	t.db.closeLk.RLock()
	defer t.db.closeLk.RUnlock()
	if t.db.closed {
		return
	}
	t.txn.Discard()
}
