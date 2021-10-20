package kv

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	badger "github.com/dgraph-io/badger/v2"
	logger "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/types"
	"go.uber.org/zap"
)

var log = logger.Logger("badger")

var ErrClosed = errors.New("datastore closed")

type compatLogger struct {
	*zap.SugaredLogger
}

// for compatibility
func (logger *compatLogger) Warningf(format string, args ...interface{}) {
	logger.Warnf(format, args...)
}

var _ types.KVStore = (*BadgerStore)(nil)

type BadgerStore struct {
	db     *badger.DB
	seqMap sync.Map

	closeLk   sync.RWMutex
	closed    bool
	closeOnce sync.Once
	closing   chan struct{}

	gcDiscardRatio float64
	gcSleep        time.Duration
	gcInterval     time.Duration

	syncWrites bool
}

// Options are the badger datastore options, reexported here for convenience.
type Options struct {
	// Please refer to the Badger docs to see what this is for
	GcDiscardRatio float64

	// Interval between GC cycles
	//
	// If zero, the datastore will perform no automatic garbage collection.
	GcInterval time.Duration

	// Sleep time between rounds of a single GC cycle.
	//
	// If zero, the datastore will only perform one round of GC per
	// GcInterval.
	GcSleep time.Duration

	badger.Options
}

// DefaultOptions are the default options for the badger datastore.
var DefaultOptions Options

func init() {
	DefaultOptions = Options{
		GcDiscardRatio: 0.5, // 0.5?
		GcInterval:     15 * time.Minute,
		GcSleep:        10 * time.Second,
		Options:        badger.DefaultOptions(""),
	}
	// This is to optimize the database on close so it can be opened
	// read-only and efficiently queried. We don't do that and hanging on
	// stop isn't nice.
	DefaultOptions.Options.CompactL0OnClose = false
}

// NewDatastore creates a new badger datastore.
//
// DO NOT set the Dir and/or ValuePath fields of opt, they will be set for you.
func NewBadgerStore(path string, options *Options) (*BadgerStore, error) {
	// Copy the options because we modify them.
	var opt badger.Options
	var gcDiscardRatio float64
	var gcSleep time.Duration
	var gcInterval time.Duration
	if options == nil {
		opt = badger.DefaultOptions("")
		gcDiscardRatio = DefaultOptions.GcDiscardRatio
		gcSleep = DefaultOptions.GcSleep
		gcInterval = DefaultOptions.GcInterval
	} else {
		opt = options.Options
		gcDiscardRatio = options.GcDiscardRatio
		gcSleep = options.GcSleep
		gcInterval = options.GcInterval
	}

	if gcSleep <= 0 {
		// If gcSleep is 0, we don't perform multiple rounds of GC per
		// cycle.
		gcSleep = gcInterval
	}

	opt.Dir = path
	opt.ValueDir = path
	opt.Logger = &compatLogger{log}

	kv, err := badger.Open(opt)
	if err != nil {
		return nil, err
	}

	ds := &BadgerStore{
		db:             kv,
		closing:        make(chan struct{}),
		gcDiscardRatio: gcDiscardRatio,
		gcSleep:        gcSleep,
		gcInterval:     gcInterval,
		syncWrites:     opt.SyncWrites,
	}

	// Start the GC process if requested.
	if ds.gcInterval > 0 {
		go ds.periodicGC()
	}

	return ds, nil
}

// Keep scheduling GC's AFTER `gcInterval` has passed since the previous GC
func (d *BadgerStore) periodicGC() {
	gcTimeout := time.NewTimer(d.gcInterval)
	defer gcTimeout.Stop()

	for {
		select {
		case <-gcTimeout.C:
			switch err := d.gcOnce(); err {
			case badger.ErrNoRewrite, badger.ErrRejected:
				// No rewrite means we've fully garbage collected.
				// Rejected means someone else is running a GC
				// or we're closing.
				gcTimeout.Reset(d.gcInterval)
			case nil:
				gcTimeout.Reset(d.gcSleep)
			case ErrClosed:
				return
			default:
				log.Errorf("error during a GC cycle: %s", err)
				// Not much we can do on a random error but log it and continue.
				gcTimeout.Reset(d.gcInterval)
			}
		case <-d.closing:
			return
		}
	}
}

func (d *BadgerStore) GetBadgerDB() *badger.DB {
	d.closeLk.RLock()
	defer d.closeLk.RUnlock()
	if d.closed {
		return nil
	}

	return d.db
}

func (d *BadgerStore) GetCounter(key []byte, bandwidth int) (*badger.Sequence, error) {
	d.closeLk.RLock()
	defer d.closeLk.RUnlock()
	if d.closed {
		return nil, ErrClosed
	}

	val, ok := d.seqMap.Load(string(key))
	if ok {
		seq, ok := val.(*badger.Sequence)
		if ok {
			return seq, nil
		}
	}

	if bandwidth == 0 {
		bandwidth = 100
	}

	newSeq, err := d.db.GetSequence(key, uint64(bandwidth))
	if err != nil {
		return nil, err
	}

	d.seqMap.Store(string(key), newSeq)

	return newSeq, nil
}

func (d *BadgerStore) GetNext(key []byte, bandwidth int) (uint64, error) {
	seq, err := d.GetCounter(key, bandwidth)
	if err != nil {
		return 0, err
	}

	return seq.Next()
}

func (d *BadgerStore) Put(key, value []byte) error {
	d.closeLk.RLock()
	defer d.closeLk.RUnlock()
	if d.closed {
		return ErrClosed
	}

	err := d.db.Update(func(txn *badger.Txn) error {
		err := txn.Set(key, value)
		return err
	})
	if err != nil {
		return err
	}

	return nil
}

// key not found is not as error
func (d *BadgerStore) Get(key []byte) (value []byte, err error) {
	d.closeLk.RLock()
	defer d.closeLk.RUnlock()
	if d.closed {
		return nil, ErrClosed
	}

	var val []byte
	err = d.db.View(func(txn *badger.Txn) error {
		switch item, err := txn.Get(key); err {
		case badger.ErrKeyNotFound:
			return nil
		case nil:
			val, err = item.ValueCopy(nil)
			return err
		default:
			return err
		}
	})
	return val, err
}

func (d *BadgerStore) Has(key []byte) (bool, error) {
	d.closeLk.RLock()
	defer d.closeLk.RUnlock()
	if d.closed {
		return false, ErrClosed
	}

	exist := false
	err := d.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get(key)
		if err != nil {
			if err != badger.ErrKeyNotFound {
				return err
			}
			return nil
		} else {
			exist = true
		}
		return err
	})
	return exist, err
}

func (d *BadgerStore) Delete(key []byte) error {
	d.closeLk.RLock()
	defer d.closeLk.RUnlock()

	err := d.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})
	return err
}

func (d *BadgerStore) Iter(prefix []byte, fn func(k, v []byte) error) int64 {
	var total int64
	d.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		opts.Prefix = prefix
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			key := item.Key()
			val, err := item.ValueCopy(nil)
			if err != nil {
				continue
			}
			if err := fn(key, val); err == nil {
				atomic.AddInt64(&total, 1)
			}
		}
		return nil
	})
	return atomic.LoadInt64(&total)
}

// iterate over keys
func (d *BadgerStore) IterKey(prefix []byte, fn func(k []byte) error) int64 {
	var total int64
	d.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // only key
		opts.Prefix = prefix
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			k := item.Key()
			if err := fn(k); err == nil {
				atomic.AddInt64(&total, 1)
			}
		}
		return nil
	})
	return atomic.LoadInt64(&total)
}

func (d *BadgerStore) Sync() error {
	d.closeLk.RLock()
	defer d.closeLk.RUnlock()
	if d.closed {
		return ErrClosed
	}

	if d.syncWrites {
		return nil
	}

	return d.db.Sync()
}

func (d *BadgerStore) Close() error {
	d.closeOnce.Do(func() {
		close(d.closing)
	})
	d.closeLk.Lock()
	defer d.closeLk.Unlock()
	if d.closed {
		return ErrClosed
	}

	d.closed = true

	d.seqMap.Range(func(k, v interface{}) bool {
		seq, ok := v.(*badger.Sequence)
		if !ok {
			return false
		}

		seq.Release()
		return true
	})
	return d.db.Close()
}

func (d *BadgerStore) CollectGarbage() (err error) {
	// The idea is to keep calling DB.RunValueLogGC() till Badger no longer has any log files
	// to GC(which would be indicated by an error, please refer to Badger GC docs).
	for err == nil {
		err = d.gcOnce()
	}

	if err == badger.ErrNoRewrite {
		err = nil
	}

	return err
}

func (d *BadgerStore) gcOnce() error {
	d.closeLk.RLock()
	defer d.closeLk.RUnlock()
	if d.closed {
		return ErrClosed
	}
	return d.db.RunValueLogGC(d.gcDiscardRatio)
}
