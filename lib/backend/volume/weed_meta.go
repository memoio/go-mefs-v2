package volume

import "encoding/binary"

type FileMeta struct {
	VolumeID uint32
	NeedleID uint64
	Cookie   uint32 // as size
}

func (f *FileMeta) Serialize() []byte {
	buf := make([]byte, 16)
	binary.BigEndian.PutUint32(buf[:4], f.VolumeID)
	binary.BigEndian.PutUint64(buf[4:12], f.NeedleID)
	binary.BigEndian.PutUint32(buf[12:16], f.Cookie)
	return buf
}

func (f *FileMeta) Deserialize(data []byte) error {
	if len(data) < 16 {
		return ErrDataLength
	}

	f.VolumeID = binary.BigEndian.Uint32(data[0:4])
	f.NeedleID = binary.BigEndian.Uint64(data[4:12])
	f.Cookie = binary.BigEndian.Uint32(data[12:16])
	return nil
}

func (w *Weed) GetFileMeta(key string, create bool) (*FileMeta, error) {
	bs, e := w.kvstore.Get([]byte(key))
	if e == nil && len(bs) > 0 {
		fm := new(FileMeta)
		err := fm.Deserialize(bs)
		if err != nil {
			return nil, err
		}

		return fm, nil
	}

	if !create {
		return nil, ErrNotFound
	}

	w.nameLock.Lock()
	defer w.nameLock.Unlock()

	next, err := w.kvstore.GetNext([]byte(needleIDKey), 100)
	if err != nil {
		return nil, err
	}

	v := w.store.GetVolume(w.volumeId)

	// size of volume file
	vsize, _, _ := v.FileStat()
	if vsize >= w.config.MaxVolumeSize {
		vid := w.volumeId + 1

		w.AddVolume(vid)
		v.MemoryMapMaxSizeMb = 0 // close previous volume
		w.volumeId = vid
	}

	f := &FileMeta{
		VolumeID: uint32(w.volumeId),
		NeedleID: next,
	}

	return f, nil
}

func (w *Weed) PutFileMeta(name string, f *FileMeta) error {
	return w.kvstore.Put([]byte(name), f.Serialize())
}
