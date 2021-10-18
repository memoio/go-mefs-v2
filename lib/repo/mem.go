package repo

import (
	"sync"

	"github.com/memoio/go-mefs-v2/config"
	"github.com/memoio/go-mefs-v2/lib/repo/fskeystore"
	"github.com/memoio/go-mefs-v2/lib/utils/blockstoreutil"

	"github.com/ipfs/go-datastore"
	dss "github.com/ipfs/go-datastore/sync"

	"github.com/memoio/go-mefs-v2/lib/utils/paths"
)

// MemRepo is an in-memory implementation of the repo interface.
type MemRepo struct {
	// lk guards the config
	lk     sync.RWMutex
	C      *config.Config
	D      blockstoreutil.Blockstore
	Ks     fskeystore.Keystore
	specDs map[string]Datastore
	W      Datastore
	Dht    Datastore
	Meta   Datastore
	Paych  Datastore
	//Market     Datastore
	version    uint
	apiAddress string
	token      []byte
}

var _ Repo = (*MemRepo)(nil)

// NewInMemoryRepo makes a new instance of MemRepo
func NewInMemoryRepo() *MemRepo {
	return &MemRepo{
		C:       config.NewDefaultConfig(),
		D:       blockstoreutil.NewBlockstore(dss.MutexWrap(datastore.NewMapDatastore())),
		Ks:      fskeystore.NewMemKeystore(),
		W:       dss.MutexWrap(datastore.NewMapDatastore()),
		Dht:     dss.MutexWrap(datastore.NewMapDatastore()),
		Meta:    dss.MutexWrap(datastore.NewMapDatastore()),
		Paych:   dss.MutexWrap(datastore.NewMapDatastore()),
		version: LatestVersion,
	}
}

// configModule returns the configuration object.
func (mr *MemRepo) Config() *config.Config {
	mr.lk.RLock()
	defer mr.lk.RUnlock()

	return mr.C
}

// ReplaceConfig replaces the current config with the newly passed in one.
func (mr *MemRepo) ReplaceConfig(cfg *config.Config) error {
	mr.lk.Lock()
	defer mr.lk.Unlock()

	mr.C = cfg

	return nil
}

// Datastore returns the datastore.
func (mr *MemRepo) Datastore() blockstoreutil.Blockstore {
	return mr.D
}

// Keystore returns the keystore.
func (mr *MemRepo) Keystore() fskeystore.Keystore {
	return mr.Ks
}

func (r *MemRepo) SpecDatastore(prefix string) Datastore {
	switch prefix {
	case dhtDatastorePrefix:
		return r.Dht
	case metaDatastorePrefix:
		return r.Meta
	default:
	}

	if ds, ok := r.specDs[prefix]; ok && ds != nil {
		return ds
	}

	ds := dss.MutexWrap(datastore.NewMapDatastore())

	r.specDs[prefix] = ds

	return ds
}

// WalletDatastore returns the wallet datastore.
func (mr *MemRepo) WalletDatastore() Datastore {
	return mr.W
}

// ChainDatastore returns the chain datastore.
func (mr *MemRepo) DhtDatastore() Datastore {
	return mr.Dht
}

// ChainDatastore returns the chain datastore.
func (mr *MemRepo) PaychDatastore() Datastore {
	return mr.Paych
}

// ChainDatastore returns the chain datastore.
func (mr *MemRepo) MetaDatastore() Datastore {
	return mr.Meta
}

// Version returns the version of the repo.
func (mr *MemRepo) Version() uint {
	return mr.version
}

// Close deletes the temporary directories which hold staged piece data and
// sealed sectors.
func (mr *MemRepo) Close() error {
	return nil
}

// SetAPIAddr writes the address of the running API to memory.
func (mr *MemRepo) SetAPIAddr(addr string) error {
	mr.apiAddress = addr
	return nil
}

// APIAddr reads the address of the running API from memory.
func (mr *MemRepo) APIAddr() (string, error) {
	return mr.apiAddress, nil
}

func (mr *MemRepo) SetAPIToken(token []byte) error {
	if len(mr.token) == 0 {
		mr.token = token
	}
	return nil
}

// Path returns the default path.
func (mr *MemRepo) Path() (string, error) {
	return paths.GetRepoPath("")
}

// JournalPath returns a string to satisfy the repo interface.
func (mr *MemRepo) JournalPath() string {
	return "in_memory_filecoin_journal_path"
}

// repo return the repo
func (mr *MemRepo) Repo() Repo {
	return mr
}
