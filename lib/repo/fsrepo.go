package repo

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
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
	"github.com/pkg/errors"
)

// Version is the version of repo schema that this code understands.
const LatestVersion uint = 3

const (
	// apiFile is the filename containing the filecoin node's api address.
	apiFile            = "api"
	configFilename     = "config.json"
	tempConfigFilename = ".config.json.temp"
	lockFile           = "repo.lock"
	versionFilename    = "version"

	keyStorePathPrefix = "keystore" // $MefsPath/keystore

	metaPathPrefix = "meta" // $MefsPath/meta

	DataPathPrefix   = "data"   // $MefsPath/data
	PiecesPathPrefix = "pieces" // $MefsPath/data/pieces
	SectorPathPrefix = "sector" // $MefsPath/data/sector
	VoluemPathPrefix = "volume" // $MefsPath/data/volume

	metaStorePrefix = "meta" // key prefix
)

var log = logging.Logger("repo")

// FSRepo is a repo implementation backed by a filesystem.
type FSRepo struct {
	// Path to the repo root directory.
	path    string
	version uint

	// lk protects the config file
	lk  sync.RWMutex
	cfg *config.Config

	keyDs  types.KeyStore
	metaDs store.KVStore
	fileDs store.FileStore

	// lockfile is the file system lock to prevent others from opening the same repo.
	lockfile io.Closer
}

var _ Repo = (*FSRepo)(nil)

// InitFSRepo initializes a new repo at the target path with the provided configuration.
// The successful result creates a symlink at targetPath pointing to a sibling directory
// named with a timestamp and repo version number.
// The link path must be empty prior. If the computed actual directory exists, it must be empty.
func InitFSRepo(targetPath string, version uint, cfg *config.Config) error {
	repoPath, err := homedir.Expand(targetPath)
	if err != nil {
		return err
	}

	if repoPath == "" { // path contained no separator
		repoPath = "./"
	}

	fmt.Println("initializing Memo repo at", repoPath)

	exists, err := fileExists(repoPath)
	if err != nil {
		return errors.Wrapf(err, "error inspecting repo path %s", repoPath)
	} else if exists {
		return errors.Errorf("repo at %s, file exists", repoPath)
	}

	// Create the actual directory and then the link to it.
	if err = InitFSRepoDirect(repoPath, version, cfg); err != nil {
		return err
	}

	return nil
}

// InitFSRepoDirect initializes a new repo at a target path, establishing a provided configuration.
// The target path must not exist, or must reference an empty, read/writable directory.
func InitFSRepoDirect(targetPath string, version uint, cfg *config.Config) error {
	repoPath, err := homedir.Expand(targetPath)
	if err != nil {
		return err
	}

	if err := ensureWritableDirectory(repoPath); err != nil {
		return errors.Wrap(err, "no writable directory")
	}

	empty, err := isEmptyDir(repoPath)
	if err != nil {
		return errors.Wrapf(err, "failed to list repo directory %s", repoPath)
	}
	if !empty {
		return fmt.Errorf("refusing to initialize repo in non-empty directory %s", repoPath)
	}

	if err := WriteVersion(repoPath, version); err != nil {
		return errors.Wrap(err, "initializing repo version failed")
	}

	if err := initConfig(repoPath, cfg); err != nil {
		return errors.Wrap(err, "initializing config file failed")
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

// OpenFSRepo opens an initialized fsrepo, expecting a specific version.
// The provided path may be to a directory, or a symbolic link pointing at a directory, which
// will be resolved just once at open.
func OpenFSRepo(repoPath string, version uint) (*FSRepo, error) {
	repoPath, err := homedir.Expand(repoPath)
	if err != nil {
		return nil, err
	}

	hasConfig, err := hasConfig(repoPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to check for repo config")
	}

	if !hasConfig {
		return nil, errors.Errorf("no repo found at %s; run: 'memo init [--repo=%s]'", repoPath, repoPath)
	}

	info, err := os.Stat(repoPath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to stat repo link %s", repoPath)
	}

	// Resolve path if it's a symlink.
	var actualPath string
	if info.IsDir() {
		actualPath = repoPath
	} else {
		actualPath, err = os.Readlink(repoPath)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to follow repo symlink %s", repoPath)
		}
	}

	r := &FSRepo{path: actualPath, version: version}

	r.lockfile, err = lockfile.Lock(r.path, lockFile)
	if err != nil {
		return nil, errors.Wrap(err, "failed to take repo lock")
	}

	if err := r.loadFromDisk(); err != nil {
		_ = r.lockfile.Close()
		return nil, err
	}

	return r, nil
}

func (r *FSRepo) loadFromDisk() error {
	localVersion, err := r.readVersion()
	if err != nil {
		return errors.Wrap(err, "failed to read version")
	}

	if localVersion > r.version {
		return fmt.Errorf("binary needs update to handle repo version, got %d expected %d. Update binary to latest release", localVersion, LatestVersion)
	}

	if err := r.loadConfig(); err != nil {
		return errors.Wrap(err, "failed to load config file")
	}

	if err := r.openKeyStore(); err != nil {
		return errors.Wrap(err, "failed to open keystore")
	}

	if err := r.openMetaStore(); err != nil {
		return errors.Wrap(err, "failed to open datastore")
	}

	if err := r.openFileStore(); err != nil {
		return errors.Wrap(err, "failed to open metadata datastore")
	}

	return nil
}

// configModule returns the configuration object.
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

func (r *FSRepo) FileStore() store.FileStore {
	return r.fileDs
}

// Version returns the version of the repo
func (r *FSRepo) Version() uint {
	return r.version
}

// Close closes the repo.
func (r *FSRepo) Close() error {
	if err := r.metaDs.Close(); err != nil {
		return errors.Wrap(err, "failed to close meta datastore")
	}

	if err := r.keyDs.Close(); err != nil {
		return errors.Wrap(err, "failed to close datastore")
	}

	if err := r.fileDs.Close(); err != nil {
		return errors.Wrap(err, "failed to close datastore")
	}

	if err := r.removeAPIFile(); err != nil {
		return errors.Wrap(err, "error removing API file")
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

// Tests whether a repo directory contains the expected config file.
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
		return errors.Wrapf(err, "failed to read config file at %q", configFile)
	}

	r.cfg = cfg
	return nil
}

// readVersion reads the repo's version file (but does not change r.version).
func (r *FSRepo) readVersion() (uint, error) {
	return ReadVersion(r.path)
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
		mpath = path.Join(r.path, DataPathPrefix)
	}

	opt := kv.DefaultOptions

	ds, err := kv.NewBadgerStore(mpath, &opt)
	if err != nil {
		return err
	}

	r.metaDs = wrap.NewKVStore(metaPathPrefix, ds)

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

// WriteVersion writes the given version to the repo version file.
func WriteVersion(p string, version uint) error {
	return ioutil.WriteFile(filepath.Join(p, versionFilename), []byte(strconv.Itoa(int(version))), 0644)
}

// ReadVersion returns the unparsed (string) version
// from the version file in the specified repo.
func ReadVersion(repoPath string) (uint, error) {
	file, err := ioutil.ReadFile(filepath.Join(repoPath, versionFilename))
	if err != nil {
		return 0, err
	}
	verStr := strings.Trim(string(file), "\n")
	version, err := strconv.ParseUint(verStr, 10, 32)
	if err != nil {
		return 0, err
	}
	return uint(version), nil
}

func initConfig(p string, cfg *config.Config) error {
	configFile := filepath.Join(p, configFilename)
	exists, err := fileExists(configFile)
	if err != nil {
		return errors.Wrap(err, "error inspecting config file")
	} else if exists {
		return fmt.Errorf("config file already exists: %s", configFile)
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
		return errors.Wrapf(err, "failed to create directory %s", path)
	}

	// Inspect existing directory.
	stat, err := os.Stat(path)
	if err != nil {
		return errors.Wrapf(err, "failed to stat path \"%s\"", path)
	}
	if !stat.IsDir() {
		return errors.Errorf("%s is not a directory", path)
	}
	if (stat.Mode() & 0600) != 0600 {
		return errors.Errorf("insufficient permissions for path %s, got %04o need %04o", path, stat.Mode(), 0600)
	}
	return nil
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
		return errors.Wrap(err, "could not create API file")
	}

	defer f.Close() // nolint: errcheck

	_, err = f.WriteString(maddr)
	if err != nil {
		// If we encounter an error writing to the API file,
		// delete the API file. The error encountered while
		// deleting the API file will be returned (if one
		// exists) instead of the write-error.
		if err := r.removeAPIFile(); err != nil {
			return errors.Wrap(err, "failed to remove API file")
		}

		return errors.Wrap(err, "failed to write to API file")
	}

	return nil
}

// Path returns the path the fsrepo is at
func (r *FSRepo) Path() (string, error) {
	return r.path, nil
}

// JournalPath returns the path the journal is at.
func (r *FSRepo) JournalPath() string {
	return fmt.Sprintf("%s/journal.json", r.path)
}

// APIAddrFromRepoPath returns the api addr from the filecoin repo
func APIAddrFromRepoPath(repoPath string) (string, error) {
	repoPath, err := homedir.Expand(repoPath)
	if err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("can't resolve local repo path %s", repoPath))
	}
	return apiAddrFromFile(repoPath)
}

// APIAddrFromFile reads the address from the API file at the given path.
// A relevant comment from a similar function at go-ipfs/repo/fsrepo/fsrepo.go:
// This is a concurrent operation, meaning that any process may read this file.
// Modifying this file, therefore, should use "mv" to replace the whole file
// and avoid interleaved read/writes
func apiAddrFromFile(repoPath string) (string, error) {
	jsonrpcFile := filepath.Join(repoPath, apiFile)
	jsonrpcAPI, err := ioutil.ReadFile(jsonrpcFile)
	if err != nil {
		return "", errors.Wrap(err, "failed to read API file")
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
