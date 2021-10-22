package repo

import (
	"github.com/memoio/go-mefs-v2/config"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

// repo is a representation of all persistent data in a filecoin node.
type Repo interface {
	Config() *config.Config
	ReplaceConfig(cfg *config.Config) error

	KeyStore() types.KeyStore   // store keyfile
	MetaStore() store.KVStore   // store meta
	FileStore() store.FileStore // store data files

	SetAPIAddr(maddr string) error
	APIAddr() (string, error)

	SetAPIToken(token []byte) error

	Version() uint

	Path() (string, error)

	Close() error

	// repo return the repo
	Repo() Repo
}
