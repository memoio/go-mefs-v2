package smt

import (
	"bytes"

	"github.com/memoio/go-mefs-v2/lib/types/store"
)

// Wrap BadgerStore, add count for value
type ValueStore struct {
	store.KVStore
}

func (vs *ValueStore) Put(key, value []byte) error {
	v, err := vs.KVStore.Get(key)
	if err != nil || len(v) > 1 && !bytes.Equal(v[1:], value) { // key not found or update
		v = append([]byte{1}, value...)
	} else { // increase count
		v[0] += 1
	}
	return vs.KVStore.Put(key, v)
}

func (vs *ValueStore) Get(key []byte) ([]byte, error) {
	v, err := vs.KVStore.Get(key)
	if err != nil {
		return v, err
	}
	if len(v) <= 1 {
		return []byte(nil), nil
	}
	return v[1:], nil
}

func (vs *ValueStore) Delete(key []byte) error {
	v, err := vs.KVStore.Get(key)
	if err != nil {
		return err
	}
	if v[0] == 1 {
		return vs.KVStore.Delete(key)
	}
	v[0] -= 1
	return vs.KVStore.Put(key, v)
}
