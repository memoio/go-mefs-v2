package storeutil

import (
	"errors"

	ds "github.com/ipfs/go-datastore"
	dsq "github.com/ipfs/go-datastore/query"

	"github.com/memoio/go-mefs-v2/lib/backend/wrap"
	"github.com/memoio/go-mefs-v2/lib/types/store"

	logging "github.com/memoio/go-mefs-v2/lib/log"
)

var logger = logging.Logger("go-datastore")

var ErrClosed = errors.New("datastore closed")

type Datastore struct {
	DB store.KVStore
}

var _ ds.Batching = (*Datastore)(nil)

// NewDatastore creates a new badger datastore.
//
// DO NOT set the Dir and/or ValuePath fields of opt, they will be set for you.
func NewDatastore(prefix string, db store.KVStore) (*Datastore, error) {
	ds := &Datastore{
		DB: wrap.NewKVStore(prefix, db),
	}

	return ds, nil
}

func (d *Datastore) Put(key ds.Key, value []byte) error {
	return d.DB.Put(key.Bytes(), value)
}

func (d *Datastore) Sync(prefix ds.Key) error {
	return nil
}

func (d *Datastore) Get(key ds.Key) ([]byte, error) {
	return d.DB.Get(key.Bytes())
}

func (d *Datastore) Has(key ds.Key) (bool, error) {
	return d.DB.Has(key.Bytes())
}

func (d *Datastore) GetSize(key ds.Key) (int, error) {
	val, err := d.DB.Get(key.Bytes())
	if err != nil {
		return 0, err
	}

	return len(val), nil
}

func (d *Datastore) Delete(key ds.Key) error {
	return d.DB.Delete(key.Bytes())
}

// todo
func (d *Datastore) Query(q dsq.Query) (dsq.Results, error) {
	logger.Debug("query: ", q.Prefix)
	qrb := dsq.NewResultBuilder(q)
	/*
		if q.KeysOnly {
			fn := func(key []byte) error {
				e := dsq.Entry{
					Key: string(key),
				}

				result := dsq.Result{Entry: e}

				if !filter(q.Filters, e) {
					qrb.Output <- result
				}

				return nil
			}

			qrb.Process.Go(func(worker goprocess.Process) {
				d.DB.IterKeys([]byte(q.Prefix), fn)
			})
		} else {
			fn := func(key, value []byte) error {
				e := dsq.Entry{
					Key:   string(key),
					Value: value,
				}

				result := dsq.Result{Entry: e}

				if !filter(q.Filters, e) {
					qrb.Output <- result
				}

				return nil
			}

			qrb.Process.Go(func(worker goprocess.Process) {
				d.DB.Iter([]byte(q.Prefix), fn)
			})
		}
	*/

	go qrb.Process.CloseAfterChildren()

	return qrb.Results(), nil
}

func filter(filters []dsq.Filter, entry dsq.Entry) bool {
	for _, f := range filters {
		if !f.Filter(entry) {
			return true
		}
	}
	return false
}

func (d *Datastore) Batch() (ds.Batch, error) {
	txns, err := d.DB.NewTxnStore(false)
	if err != nil {
		return nil, err
	}

	return &txn{d, txns}, nil
}

// DiskUsage implements the PersistentDatastore interface.
// It returns the sum of lsm and value log files sizes in bytes.
func (d *Datastore) DiskUsage() (uint64, error) {
	return 0, nil
}

func (d *Datastore) Close() error {
	return d.DB.Close()
}

type txn struct {
	ds  *Datastore
	txn store.TxnStore
}

func (t *txn) Put(key ds.Key, value []byte) error {
	return t.txn.Put(key.Bytes(), value)
}

func (t *txn) Delete(key ds.Key) error {
	return t.txn.Delete(key.Bytes())
}

func (t *txn) Commit() error {
	return t.txn.Commit()
}
