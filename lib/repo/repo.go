package repo

import (
	"github.com/ipfs/go-datastore"

	"github.com/memoio/go-mefs-v2/config"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

// Datastore is the datastore interface provided by the repo
type Datastore interface {
	// NB: there are other more featureful interfaces we could require here, we
	// can either force it, or just do hopeful type checks. Not all datastores
	// implement every feature.
	datastore.Batching
}

// repo is a representation of all persistent data in a filecoin node.
type Repo interface {
	Config() *config.Config

	// ReplaceConfig replaces the current config, with the newly passed in one.
	ReplaceConfig(cfg *config.Config) error

	MetaStore() store.KVStore

	FileStore() store.FileStore

	Keystore() types.KeyStore

	// DhtDatastore is a specific storage solution, only used to store kad dht data.
	DhtDatastore() Datastore

	// SetJsonrpcAPIAddr sets the address of the running jsonrpc API.
	SetAPIAddr(maddr string) error

	// APIAddr returns the address of the running API.
	APIAddr() (string, error)

	// SetAPIToken set api token
	SetAPIToken(token []byte) error

	// Version returns the current repo version.
	Version() uint

	// Path returns the repo path.
	Path() (string, error)

	// Close shuts down the repo.
	Close() error

	// repo return the repo
	Repo() Repo
}
