package simplefs

import (
	"io/ioutil"
	"os"
	"path"

	"golang.org/x/xerrors"

	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/lib/utils"
)

var logger = logging.Logger("simplefs")

var _ store.FileStore = (*SimpleFs)(nil)

type SimpleFs struct {
	size    int64
	basedir string
}

func NewSimpleFs(dir string) (*SimpleFs, error) {
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return nil, err
	}

	sf := &SimpleFs{0, dir}
	sf.walk(dir)

	logger.Info("create simplefs at:", dir)

	return sf, nil
}

func (sf *SimpleFs) walk(baseDir string) {
	rd, err := ioutil.ReadDir(baseDir)
	if err != nil {
		return
	}

	for _, fi := range rd {
		if fi.IsDir() {
			sf.walk(path.Join(baseDir, fi.Name()))
		} else {
			sf.size += fi.Size()
		}
	}
}

func (sf *SimpleFs) Put(key, val []byte) error {
	skey := string(key)

	dir := path.Join(sf.basedir, path.Dir(skey))

	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return err
	}

	fn := path.Join(dir, path.Base(skey))

	info, err := os.Stat(fn)
	if err == nil && !info.IsDir() {
		err := os.Remove(fn)
		if err != nil {
			return err
		}

		sf.size -= info.Size()
	}

	err = ioutil.WriteFile(fn, val, 0644)
	if err != nil {
		return err
	}

	sf.size += int64(len(val))

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
		return false, xerrors.New("is a dir")
	}

	return true, nil
}

func (sf *SimpleFs) Delete(key []byte) error {
	skey := string(key)

	dir := path.Join(sf.basedir, path.Dir(skey))
	fn := path.Join(dir, path.Base(skey))

	fi, err := os.Stat(fn)
	if err != nil {
		return err
	}

	err = os.Remove(fn)
	if err != nil {
		return err
	}

	if !fi.IsDir() {
		sf.size -= fi.Size()
	}

	return nil
}

func (sf *SimpleFs) Size() store.DiskStats {
	ds, _ := utils.GetDiskStatus(sf.basedir)
	if sf.size > 0 {
		ds.Used = uint64(sf.size)
	} else {
		ds.Used = 0
	}

	return ds
}

func (sf *SimpleFs) Close() error {
	return nil
}
