package smt

import (
	"bytes"

	"github.com/memoio/go-mefs-v2/lib/types/store"
)

// Wrap BadgerStore, add count for value
type ValueStore struct {
	store.KVStore
}

func (vs *ValueStore) NewTxnStore(update bool) (store.TxnStore, error) {
	txn, err := vs.KVStore.NewTxnStore(update)
	if err != nil {
		return nil, err
	}

	return &valueTxn{txn}, nil
}

func (vs *ValueStore) Put(key, value []byte) error {
	return wrappedPut(key, value, vs.KVStore)
}

func (vs *ValueStore) Get(key []byte) ([]byte, error) {
	return wrappedGet(key, vs.KVStore)
}

func (vs *ValueStore) Delete(key []byte) error {
	return wrappedDelete(key, vs.KVStore)
}

// Wrap Txn, add count for value
type valueTxn struct {
	store.TxnStore
}

func (t *valueTxn) Put(key, value []byte) error {
	return wrappedPut(key, value, t.TxnStore)
}

func (t *valueTxn) Get(key []byte) ([]byte, error) {
	return wrappedGet(key, t.TxnStore)
}

func (t *valueTxn) Delete(key []byte) error {
	return wrappedDelete(key, t.TxnStore)
}

// wrapped operation
func wrappedPut(key, value []byte, s store.Store) error {
	v, err := s.Get(key)
	if err != nil || len(v) > 1 && !bytes.Equal(v[1:], value) { // key not found or update
		v = append([]byte{1}, value...)
	} else { // increase count
		v[0] += 1
	}
	return s.Put(key, v)
}

func wrappedGet(key []byte, s store.Store) ([]byte, error) {
	v, err := s.Get(key)
	if err != nil {
		return v, err
	}
	if len(v) <= 1 {
		return []byte(nil), nil
	}
	return v[1:], nil
}

func wrappedDelete(key []byte, s store.Store) error {
	v, err := s.Get(key)
	if err != nil {
		return err
	}
	if v[0] == 1 {
		return s.Delete(key)
	}
	v[0] -= 1
	return s.Put(key, v)
}
