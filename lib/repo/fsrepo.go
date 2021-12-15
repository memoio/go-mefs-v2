package repo

import (
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sync"

	"github.com/memoio/go-mefs-v2/config"
	"github.com/memoio/go-mefs-v2/lib/backend/keystore"
	"github.com/memoio/go-mefs-v2/lib/backend/kv"
	"github.com/memoio/go-mefs-v2/lib/backend/simplefs"
	"github.com/memoio/go-mefs-v2/lib/backend/wrap"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"

	lockfile "github.com/ipfs/go-fs-lock"
	"github.com/mitchellh/go-homedir"
	"golang.org/x/xerrors"
)

const (
	apiFile            = "api"
	configFilename     = "config.json"
	tempConfigFilename = ".config.json.temp"
	lockFile           = "repo.lock"
	versionFilename    = "version"

	keyStorePathPrefix = "keystore" // $MefsPath/keystore

	metaPathPrefix  = "meta"  // $MefsPath/meta
	statePathPrefix = "state" // $MefsPath/state

	DataPathPrefix   = "data"   // $MefsPath/data
	PiecesPathPrefix = "pieces" // $MefsPath/data/pieces
	SectorPathPrefix = "sector" // $MefsPath/data/sector
	VoluemPathPrefix = "volume" // $MefsPath/data/volume

	metaStorePrefix  = "meta" // key prefix
	stateStorePrefix = "meta" // key prefix
)

var logger = logging.Logger("repo")

// FSRepo is a repo implementation backed by a filesystem.
type FSRepo struct {
	// Path to the repo root directory.
	path string

	// lk protects the config file
	lk  sync.RWMutex
	cfg *config.Config

	keyDs   types.KeyStore
	metaDs  store.KVStore
	stateDs store.KVStore
	fileDs  store.FileStore

	// lockfile is the file system lock to prevent others from opening the same repo.
	lockfile io.Closer
}

var _ Repo = (*FSRepo)(nil)

func NewFSRepo(dir string, cfg *config.Config) (*FSRepo, error) {
	repoPath, err := homedir.Expand(dir)
	if err != nil {
		return nil, err
	}

	if repoPath == "" { // path contained no separator
		repoPath = "./"
	}

	if err := ensureWritableDirectory(repoPath); err != nil {
		return nil, xerrors.Errorf("no writable directory %w", err)
	}

	hasConfig, err := hasConfig(repoPath)
	if err != nil {
		return nil, xerrors.Errorf("failed to check for repo config %w", err)
	}

	if !hasConfig {
		if cfg != nil {
			logger.Info("initializing memo repo at: ", repoPath)
			if err = initFSRepo(repoPath, cfg); err != nil {
				return nil, err
			}
		} else {
			return nil, xerrors.Errorf("no repo found at %s; run: 'init [--repo=%s]'", repoPath, repoPath)
		}
	}

	info, err := os.Stat(repoPath)
	if err != nil {
		return nil, xerrors.Errorf("failed to stat repo %s %w", repoPath, err)
	}

	// Resolve path if it's a symlink.
	var actualPath string
	if info.IsDir() {
		actualPath = repoPath
	} else {
		actualPath, err = os.Readlink(repoPath)
		if err != nil {
			return nil, xerrors.Errorf("failed to follow repo symlink %s %w", repoPath, err)
		}
	}

	r := &FSRepo{path: actualPath}

	r.lockfile, err = lockfile.Lock(r.path, lockFile)
	if err != nil {
		return nil, xerrors.Errorf("failed to take repo lock %w", err)
	}

	if err := r.loadFromDisk(); err != nil {
		_ = r.lockfile.Close()
		return nil, err
	}

	logger.Info("open repo at:", repoPath)

	return r, nil
}

func initFSRepo(dir string, cfg *config.Config) error {
	if err := initConfig(dir, cfg); err != nil {
		return xerrors.Errorf("initializing config file failed %w", err)
	}

	kstorePath := filepath.Join(dir, keyStorePathPrefix)
	if err := os.MkdirAll(kstorePath, 0700); err != nil {
		return xerrors.Errorf("initializing keystore directory failed %w", err)
	}

	return nil
}

func (r *FSRepo) loadFromDisk() error {
	if err := r.loadConfig(); err != nil {
		return xerrors.Errorf("failed to load config file %w", err)
	}

	if err := r.openKeyStore(); err != nil {
		return xerrors.Errorf("failed to open keystore %w", err)
	}

	if err := r.openMetaStore(); err != nil {
		return xerrors.Errorf("failed to open meta store %w", err)
	}

	if err := r.openStateStore(); err != nil {
		return xerrors.Errorf("failed to open state store %w", err)
	}

	if err := r.openFileStore(); err != nil {
		return xerrors.Errorf("failed to open metadata datastore %w", err)
	}

	return nil
}

func (r *FSRepo) Config() *config.Config {
	r.lk.RLock()
	defer r.lk.RUnlock()

	return r.cfg
}

// ReplaceConfig replaces the current config with the newly passed in one.
func (r *FSRepo) ReplaceConfig(cfg *config.Config) error {
	r.lk.Lock()
	defer r.lk.Unlock()

	r.cfg = cfg
	tmp := filepath.Join(r.path, tempConfigFilename)
	err := os.RemoveAll(tmp)
	if err != nil {
		return err
	}
	err = r.cfg.WriteFile(tmp)
	if err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(r.path, configFilename))
}

func (r *FSRepo) KeyStore() types.KeyStore {
	return r.keyDs
}

func (r *FSRepo) MetaStore() store.KVStore {
	return r.metaDs
}

func (r *FSRepo) StateStore() store.KVStore {
	return r.stateDs
}

func (r *FSRepo) FileStore() store.FileStore {
	return r.fileDs
}

// Close closes the repo.
func (r *FSRepo) Close() error {
	if err := r.metaDs.Close(); err != nil {
		return xerrors.Errorf("failed to close meta datastore %w", err)
	}

	if err := r.stateDs.Close(); err != nil {
		return xerrors.Errorf("failed to close meta datastore %w", err)
	}

	if err := r.keyDs.Close(); err != nil {
		return xerrors.Errorf("failed to close datastore %w", err)
	}

	if err := r.fileDs.Close(); err != nil {
		return xerrors.Errorf("failed to close file store %w", err)
	}

	if err := r.removeAPIFile(); err != nil {
		return xerrors.Errorf("failed to remove API file %w", err)
	}

	return r.lockfile.Close()
}

func (r *FSRepo) removeFile(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

func (r *FSRepo) removeAPIFile() error {
	return r.removeFile(filepath.Join(r.path, apiFile))
}

func hasConfig(p string) (bool, error) {
	configPath := filepath.Join(p, configFilename)

	_, err := os.Lstat(configPath)
	switch {
	case err == nil:
		return true, nil
	case os.IsNotExist(err):
		return false, nil
	default:
		return false, err
	}
}

func (r *FSRepo) loadConfig() error {
	configFile := filepath.Join(r.path, configFilename)

	cfg, err := config.ReadFile(configFile)
	if err != nil {
		return xerrors.Errorf("failed to read config file at %q %w", configFile, err)
	}

	r.cfg = cfg
	return nil
}

func (r *FSRepo) openKeyStore() error {
	ksp := filepath.Join(r.path, "keystore")

	ks, err := keystore.NewKeyRepo(ksp)
	if err != nil {
		return err
	}

	r.keyDs = ks

	return nil
}

func (r *FSRepo) openMetaStore() error {
	mpath := r.cfg.Data.MetaPath
	if mpath == "" {
		mpath = path.Join(r.path, metaPathPrefix)
	}

	opt := kv.DefaultOptions

	ds, err := kv.NewBadgerStore(mpath, &opt)
	if err != nil {
		return err
	}

	r.metaDs = wrap.NewKVStore(metaStorePrefix, ds)

	return nil
}

func (r *FSRepo) openStateStore() error {
	mpath := path.Join(r.path, statePathPrefix)

	opt := kv.DefaultOptions

	ds, err := kv.NewBadgerStore(mpath, &opt)
	if err != nil {
		return err
	}

	r.stateDs = wrap.NewKVStore(stateStorePrefix, ds)

	return nil
}

func (r *FSRepo) openFileStore() error {
	dpath := r.cfg.Data.DataPath
	if dpath == "" {
		dpath = path.Join(r.path, DataPathPrefix)
	}

	ds, err := simplefs.NewSimpleFs(dpath)
	if err != nil {
		return err
	}

	r.fileDs = ds

	return nil
}

func initConfig(p string, cfg *config.Config) error {
	configFile := filepath.Join(p, configFilename)
	exists, err := fileExists(configFile)
	if err != nil {
		return xerrors.Errorf("failed to inspect config file %w", err)
	} else if exists {
		return xerrors.Errorf("config file already exists: %s", configFile)
	}

	if err := cfg.WriteFile(configFile); err != nil {
		return err
	}

	// make the snapshot dir
	return nil
}

// Ensures that path points to a read/writable directory, creating it if necessary.
func ensureWritableDirectory(path string) error {
	// Attempt to create the requested directory, accepting that something might already be there.
	err := os.Mkdir(path, 0775)

	if err == nil {
		return nil // Skip the checks below, we just created it.
	} else if !os.IsExist(err) {
		return xerrors.Errorf("failed to create directory %s %w", path, err)
	}

	// Inspect existing directory.
	stat, err := os.Stat(path)
	if err != nil {
		return xerrors.Errorf("failed to stat path %s %w", path, err)
	}
	if !stat.IsDir() {
		return xerrors.Errorf("%s is not a directory", path)
	}
	if (stat.Mode() & 0600) != 0600 {
		return xerrors.Errorf("insufficient permissions for path %s, got %04o need %04o", path, stat.Mode(), 0600)
	}
	return nil
}

func Exists(repoPath string) (bool, error) {
	_, err := os.Stat(filepath.Join(repoPath, keyStorePathPrefix))
	notExist := os.IsNotExist(err)
	if notExist {
		err = nil

		_, err = os.Stat(filepath.Join(repoPath, configFilename))
		notExist = os.IsNotExist(err)
		if notExist {
			err = nil
		}
	}
	return !notExist, err
}

// Tests whether the directory at path is empty
func isEmptyDir(path string) (bool, error) {
	infos, err := ioutil.ReadDir(path)
	if err != nil {
		return false, err
	}
	return len(infos) == 0, nil
}

func fileExists(file string) (bool, error) {
	_, err := os.Stat(file)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// SetAPIAddr writes the address to the API file. SetAPIAddr expects parameter
// `port` to be of the form `:<port>`.
func (r *FSRepo) SetAPIAddr(maddr string) error {
	f, err := os.Create(filepath.Join(r.path, apiFile))
	if err != nil {
		return xerrors.Errorf("could not create API file %w", err)
	}

	defer f.Close() // nolint: errcheck

	_, err = f.WriteString(maddr)
	if err != nil {
		if err := r.removeAPIFile(); err != nil {
			return xerrors.Errorf("failed to remove API file %w", err)
		}

		return xerrors.Errorf("failed to write to API file %w", err)
	}

	return nil
}

// Path returns the path the fsrepo is at
func (r *FSRepo) Path() (string, error) {
	return r.path, nil
}

func apiAddrFromFile(repoPath string) (string, error) {
	jsonrpcFile := filepath.Join(repoPath, apiFile)
	jsonrpcAPI, err := ioutil.ReadFile(jsonrpcFile)
	if err != nil {
		return "", xerrors.Errorf("failed to read API file %w", err)
	}

	return string(jsonrpcAPI), nil
}

// APIAddr reads the FSRepo's api file and returns the api address
func (r *FSRepo) APIAddr() (string, error) {
	return apiAddrFromFile(filepath.Clean(r.path))
}

func (r *FSRepo) SetAPIToken(token []byte) error {
	return ioutil.WriteFile(filepath.Join(r.path, "token"), token, 0600)
}

func (r *FSRepo) Repo() Repo {
	return r
}
