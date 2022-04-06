package repo

import (
	"context"

	"github.com/ipfs/go-datastore"

	"github.com/memoio/go-mefs-v2/config"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

type Repo interface {
	Config() *config.Config
	ReplaceConfig(cfg *config.Config) error

	KeyStore() types.KeyStore   // store keyfile
	MetaStore() store.KVStore   // store meta
	StateStore() store.KVStore  // store state meta
	FileStore() store.FileStore // store data files

	DhtStore() datastore.Batching // for dht

	SetAPIAddr(maddr string) error
	APIAddr() (string, error)

	SetAPIToken(token []byte) error

	Path() (string, error)

	LocalStoreGetMeta(context.Context) (store.DiskStats, error)
	LocalStoreGetData(context.Context) (store.DiskStats, error)

	Close() error

	// repo return the repo
	Repo() Repo
}
