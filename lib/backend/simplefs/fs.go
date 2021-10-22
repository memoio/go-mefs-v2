package simplefs

import (
	"errors"
	"io/ioutil"
	"os"
	"path"

	logger "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

var log = logger.Logger("simplefs")

var _ store.FileStore = (*SimpleFs)(nil)

type SimpleFs struct {
	basedir string
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

func (sf *SimpleFs) Put(key, val []byte) error {
	skey := string(key)

	dir := path.Join(sf.basedir, path.Dir(skey))

	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return err
	}

	fn := path.Join(dir, path.Base(skey))

	err = ioutil.WriteFile(fn, val, 0644)
	if err != nil {
		return err
	}

	return nil
}

func (sf *SimpleFs) Get(key []byte) ([]byte, error) {
	skey := string(key)

	dir := path.Join(sf.basedir, path.Dir(skey))
	fn := path.Join(dir, path.Base(skey))

	data, err := ioutil.ReadFile(fn)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (sf *SimpleFs) Has(key []byte) (bool, error) {
	skey := string(key)

	dir := path.Join(sf.basedir, path.Dir(skey))
	fn := path.Join(dir, path.Base(skey))

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
	skey := string(key)

	dir := path.Join(sf.basedir, path.Dir(skey))
	fn := path.Join(dir, path.Base(skey))

	return os.Remove(fn)
}

func (sf *SimpleFs) Size() int64 {
	return 0
}

func (sf *SimpleFs) Close() error {
	return nil
}
