package simplefs

import (
	"encoding/binary"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/xerrors"

	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/lib/utils"
)

var logger = logging.Logger("simplefs")

var _ store.FileStore = (*SimpleFs)(nil)

const usage = "usage"

type SimpleFs struct {
	sync.RWMutex
	size    int64
	basedir string
}

func NewSimpleFs(dir string) (*SimpleFs, error) {
	logger.Infof("start simplefs at: %s", dir)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return nil, err
	}

	sf := &SimpleFs{
		basedir: dir,
	}

	ub, err := ioutil.ReadFile(filepath.Join(dir, usage))
	if err == nil && len(ub) >= 8 {
		sf.size = int64(binary.BigEndian.Uint64(ub))
	} else {
		logger.Infof("calculate usage by walking simplefs: %s", dir)
		sf.walk(dir)
		sf.writeSize()
	}

	logger.Infof("create simplefs at: %s %d", dir, sf.size)

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
			continue
		}

		if strings.HasSuffix(fi.Name(), ".tmp") {
			os.Remove(fi.Name())
			continue
		}

		if !strings.Contains(fi.Name(), "_") {
			continue
		}

		sf.size += fi.Size()
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

		sf.Lock()
		sf.size -= info.Size()
		sf.Unlock()
	}

	// write then rename
	tmpfn := fn + ".tmp"
	err = ioutil.WriteFile(tmpfn, val, 0644)
	if err != nil {
		return err
	}
	err = os.Rename(tmpfn, fn)
	if err != nil {
		return err
	}

	sf.Lock()
	sf.size += int64(len(val))
	sf.writeSize()
	sf.Unlock()

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
		sf.Lock()
		sf.size -= fi.Size()
		sf.writeSize()
		sf.Unlock()
	}

	return nil
}

func (sf *SimpleFs) Size() store.DiskStats {
	ds, _ := utils.GetDiskStatus(sf.basedir)
	if sf.size >= 0 {
		ds.Used = uint64(sf.size)
	}

	return ds
}

func (sf *SimpleFs) writeSize() error {
	ub := make([]byte, 8)
	binary.BigEndian.PutUint64(ub, uint64(sf.size))

	return ioutil.WriteFile(filepath.Join(sf.basedir, usage), ub, 0644)
}

func (sf *SimpleFs) Close() error {
	sf.Lock()
	defer sf.Unlock()
	return sf.writeSize()
}
