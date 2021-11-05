package volume

import (
	"encoding/json"
	"sync"
	"time"

	ws "github.com/chrislusf/seaweedfs/weed/storage"
	"github.com/chrislusf/seaweedfs/weed/storage/needle"
	wtype "github.com/chrislusf/seaweedfs/weed/storage/types"
	"github.com/mr-tron/base58/base58"

	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

var logger = logging.Logger("volume")

var _ store.FileStore = (*Weed)(nil)

type Weed struct {
	config   *Config
	store    *ws.Store
	kvstore  store.KVStore
	volumeId needle.VolumeId
	nameLock sync.RWMutex

	closed    bool
	closeLk   sync.RWMutex
	closeOnce sync.Once
	closing   chan struct{}
}

func NewWeed(cfg *Config, kvstore store.KVStore) (store.FileStore, error) {
	if cfg == nil {
		return nil, ErrEmptyConfig
	}

	store := ws.NewStore(
		nil,
		"",
		10240, // port, not used
		10241,
		"127.0.0.1",
		cfg.Dirnames,
		cfg.MaxVolumeCounts,
		cfg.MinFreeSpaces,
		cfg.IdxFolder,
		ws.NeedleMapLevelDbMedium,
		cfg.DiskType,
	)

	var volumeId needle.VolumeId
	vid, err := kvstore.Get([]byte(volumeIDKey))
	if err != nil || len(vid) == 0 {
		volumeId = initialVolumeId
		err = kvstore.Put([]byte(volumeIDKey), []byte(volumeId.String()))
		if err != nil {
			return nil, err
		}
	} else {
		volumeId, err = needle.NewVolumeId(string(vid))
		if err != nil {
			return nil, err
		}
	}

	w := &Weed{
		config:   cfg,
		store:    store,
		kvstore:  kvstore,
		volumeId: volumeId,
		closing:  make(chan struct{}),
	}

	w.AddVolume(volumeId)

	go w.periodicGC()

	return w, nil
}

func (w *Weed) AddLocation(path string) error {
	w.closeLk.RLock()
	defer w.closeLk.RUnlock()
	if w.closed {
		return ErrClosed
	}

	err := w.config.AddPath(path)
	if err != nil {
		return err
	}

	cByte, err := json.Marshal(w.config)
	if err != nil {
		return err
	}

	err = w.kvstore.Put([]byte(volumeConfigKey), cByte)
	if err != nil {
		return err
	}

	dl := ws.NewDiskLocation(path, defaultVolumeCount, defaultMinSpace, w.config.IdxFolder, wtype.HardDriveType)
	// need lock?
	w.store.Locations = append(w.store.Locations, dl)

	// put to
	return nil
}

func (w *Weed) AddVolume(volumeId needle.VolumeId) error {
	v := w.store.HasVolume(volumeId)
	if !v {
		logger.Info("add volume: ", uint32(volumeId))
		err := w.store.AddVolume(volumeId, collection, ws.NeedleMapLevelDbMedium, "000", "", 128, 128, wtype.HardDriveType)
		if err != nil {
			return err
		}
		<-w.store.NewVolumesChan
		err = w.kvstore.Put([]byte(volumeIDKey), []byte(volumeId.String()))
		if err != nil {
			return err
		}
		logger.Info("added volume: ", uint32(volumeId))
	}

	return nil
}

func (w *Weed) DeleteVolume(volumeId needle.VolumeId) error {
	v := w.store.HasVolume(volumeId)
	if !v {
		logger.Info("delete volume: ", uint32(volumeId))
		err := w.store.DeleteVolume(volumeId)
		if err != nil {
			return err
		}
		<-w.store.DeletedVolumesChan
		logger.Info("deleted volume: ", uint32(volumeId))
	}

	return nil
}

func toString(key []byte) string {
	return base58.Encode(key)
}

func keyToByte(str string) []byte {
	kbyte, err := base58.Decode(str)
	if err != nil {
		return nil
	}

	return kbyte
}

func (w *Weed) Put(key, value []byte) error {
	w.closeLk.RLock()
	defer w.closeLk.RUnlock()
	if w.closed {
		return ErrClosed
	}

	fm, err := w.GetFileMeta(toString(key), true)
	if err != nil {
		return err
	}

	vLen := len(value)

	n := &needle.Needle{
		Id:       wtype.NeedleId(fm.NeedleID),
		Data:     value,
		Checksum: needle.NewCRC(value),
		Cookie:   wtype.Cookie(vLen),
	}

	_, err = w.store.WriteVolumeNeedle(needle.VolumeId(fm.VolumeID), n, true, true)
	if err != nil {
		return err
	}

	v := w.store.GetVolume(needle.VolumeId(fm.VolumeID))
	dsize, iSize, _ := v.FileStat()
	logger.Debug("after write: ", uint64(n.Id), dsize, iSize, v.ContentSize())

	if vLen != int(fm.Cookie) {
		fm.Cookie = uint32(vLen)
		w.PutFileMeta(toString(key), fm)
	}

	return nil
}

func (w *Weed) Get(key []byte) ([]byte, error) {
	w.closeLk.RLock()
	defer w.closeLk.RUnlock()
	if w.closed {
		return nil, ErrClosed
	}

	fm, err := w.GetFileMeta(toString(key), false)
	if err != nil {
		return nil, err
	}

	n := new(needle.Needle)
	n.Id = wtype.NeedleId(fm.NeedleID)
	n.Cookie = wtype.Cookie(fm.Cookie)

	count, err := w.store.ReadVolumeNeedle(needle.VolumeId(fm.VolumeID), n, nil, nil)
	if err != nil {
		return nil, err
	}

	if count < 0 {
		return nil, ErrNotFound
	}

	return n.Data, nil
}

func (w *Weed) Has(key []byte) (bool, error) {
	w.closeLk.RLock()
	defer w.closeLk.RUnlock()
	if w.closed {
		return false, ErrClosed
	}

	fm, err := w.GetFileMeta(toString(key), false)
	if err != nil {
		return false, err
	}

	if fm == nil {
		return false, nil
	}

	return true, nil
}

func (w *Weed) Delete(key []byte) error {
	w.closeLk.RLock()
	defer w.closeLk.RUnlock()
	if w.closed {
		return ErrClosed
	}

	fm, err := w.GetFileMeta(toString(key), false)
	if err != nil {
		return err
	}

	n := new(needle.Needle)
	n.Id = wtype.NeedleId(fm.NeedleID)
	n.Cookie = wtype.Cookie(fm.Cookie)

	_, err = w.store.DeleteVolumeNeedle(needle.VolumeId(fm.VolumeID), n)
	if err != nil {
		return err
	}

	// update size

	return w.kvstore.Delete([]byte(key))
}

func (w *Weed) Size() int64 {
	w.closeLk.RLock()
	defer w.closeLk.RUnlock()
	if w.closed {
		return 0
	}

	maxvid := int(w.volumeId)

	st := uint64(0)

	for i := 0; i <= maxvid; i++ {
		v := w.store.GetVolume(needle.VolumeId(i))

		fSize, iSize, _ := v.FileStat()
		st += fSize
		st += iSize
	}

	return int64(st)
}

func (w *Weed) Stat() (*store.Statistics, error) {
	w.closeLk.RLock()
	defer w.closeLk.RUnlock()
	if w.closed {
		return nil, ErrClosed
	}

	maxvid := int(w.volumeId)

	st := new(store.Statistics)

	for i := 0; i <= maxvid; i++ {
		v := w.store.GetVolume(needle.VolumeId(i))
		st.ContentSize += v.ContentSize()
		fSize, iSize, _ := v.FileStat()
		st.Size += fSize
		st.Size += iSize

		st.Count += v.FileCount()
	}

	logger.Info("stats: ", st)
	return st, nil
}

func (w *Weed) Close() error {
	w.closeOnce.Do(func() {
		close(w.closing)
	})

	w.closeLk.Lock()
	defer w.closeLk.Unlock()

	if w.closed {
		return ErrClosed
	}

	w.closed = true
	w.store.Close()

	return nil
}

func (w *Weed) periodicGC() {
	gcTimeout := time.NewTimer(gcInterval)
	defer gcTimeout.Stop()

	for {
		select {
		case <-gcTimeout.C:
			err := w.GarbageCollect(false)
			if err == ErrClosed {
				return
			}
		case <-w.closing:
			return
		}
	}
}

func (w *Weed) GarbageCollect(all bool) error {
	w.closeLk.RLock()
	defer w.closeLk.RUnlock()
	if w.closed {
		return ErrClosed
	}

	for i := 0; i < int(w.volumeId); i++ {
		vid := needle.VolumeId(i)
		gclevel, err := w.store.CheckCompactVolume(vid)
		if err != nil {
			continue
		}

		if gclevel > 0.2 {
			v := w.store.GetVolume(vid)
			err := w.store.CompactVolume(vid, 128*1024*1024, 10*1024*1024)
			logger.Info("compact volume: ", i, gclevel, v.MemoryMapMaxSizeMb, err)
			if err != nil {
				err = w.store.CommitCleanupVolume(vid)
				logger.Info("commmit clenaup volume: ", i, err)
				if err != nil {
					continue
				}
			} else {
				ok, err := w.store.CommitCompactVolume(vid)
				logger.Info("commmit compact volume: ", i, ok, err)
				if err != nil {
					continue
				}
			}

			if all {
				continue
			}
		}
	}

	return nil
}
