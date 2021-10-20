package simplefs

import (
	"encoding/hex"
	"errors"
	"io/ioutil"
	"os"
	"path"

	logger "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/mr-tron/base58/base58"
	"github.com/zeebo/blake3"
)

var log = logger.Logger("simplefs")

var _ types.FileStore = (*SimpleFs)(nil)

type SimpleFs struct {
	path string
}

func NewSimpleFs(dir string) (*SimpleFs, error) {
	stat, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !stat.IsDir() {
		return nil, errors.New("not dir")
	}

	log.Info("create simplefs at:", dir)

	return &SimpleFs{dir}, nil
}

func getFileName(key []byte) string {
	if len(key) > 20 {
		return base58.Encode(key[20:])
	} else {
		return base58.Encode(key)
	}
}

func getFilePath(baseDir string, key []byte) string {
	fn := baseDir
	if len(key) > 20 {
		fn = path.Join(fn, base58.Encode(key[:20]))
	}

	h := blake3.Sum256([]byte(key))
	x := hex.EncodeToString(h[:])
	fn = path.Join(fn, x[len(x)-2:])

	fn = path.Join(fn, x[len(x)-2:])
	return fn
}

func createFilePath(baseDir string, key []byte) (string, error) {
	fn := baseDir
	if len(key) > 20 {
		fn = path.Join(fn, base58.Encode(key[:20]))
		if err := os.Mkdir(fn, 0755); err != nil {
			if !os.IsExist(err) {
				return "", err
			}
		}
	}

	h := blake3.Sum256([]byte(key))
	x := hex.EncodeToString(h[:])
	fn = path.Join(fn, x[len(x)-2:])

	if err := os.Mkdir(fn, 0755); err != nil {
		if !os.IsExist(err) {
			return "", err
		}
	}

	info, err := os.Stat(fn)
	if err != nil {
		return "", err
	}

	if !info.IsDir() {
		return "", errors.New("not a dir")
	}

	return fn, nil
}

func (sf *SimpleFs) Put(key, val []byte) error {
	dir, err := createFilePath(sf.path, key)
	if err != nil {
		return err
	}

	basename := getFileName(key)

	fn := path.Join(dir, basename)

	err = ioutil.WriteFile(fn, val, 0644)
	if err != nil {
		return err
	}

	return nil
}

func (sf *SimpleFs) Get(key []byte) ([]byte, error) {
	dir := getFilePath(sf.path, key)

	basename := getFileName(key)

	fn := path.Join(dir, basename)

	data, err := ioutil.ReadFile(fn)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (sf *SimpleFs) Has(key []byte) (bool, error) {
	dir := getFilePath(sf.path, key)

	basename := getFileName(key)

	fn := path.Join(dir, basename)

	info, err := os.Stat(fn)
	if err != nil || os.IsNotExist(err) {
		return false, err
	}

	if info.IsDir() {
		return false, errors.New("is a dir")
	}

	return true, nil
}

func (sf *SimpleFs) Delete(key []byte) error {
	dir := getFilePath(sf.path, key)

	basename := getFileName(key)

	fn := path.Join(dir, basename)
	return os.Remove(fn)
}

func (sf *SimpleFs) Stat() (*types.Statistics, error) {
	return nil, nil
}

func (sf *SimpleFs) Close() error {
	return nil
}
