package order

import (
	"encoding/binary"
	"sync"

	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/lib/utils"
)

type lastProvider struct {
	sync.RWMutex
	bucketID uint64
	dc, pc   int
	pros     []uint64 // pre pros; update and save to local
}

func (m *OrderMgr) RegisterBucket(bucketID uint64, bopt *pb.BucketOption) {
	_, ok := m.proMap[bucketID]
	if ok {
		return
	}

	pros := make([]uint64, 0, int(bopt.DataCount+bopt.ParityCount))
	storedPros := m.loadLastProviders(bucketID)
	pros = append(pros, storedPros...)

	// add missing
	if len(storedPros) < int(bopt.DataCount+bopt.ParityCount) {
		// sort
		proValue := m.proSet.Values()
		utils.Disorder(proValue)
		for _, pid := range proValue {
			has := false
			for _, hasPid := range storedPros {
				if pid == hasPid {
					has = true
				}
			}

			if !has {
				pros = append(pros, pid.(uint64))
			}

			if len(pros) == int(bopt.DataCount+bopt.ParityCount) {
				break
			}
		}
	}

	if len(pros) != int(bopt.DataCount+bopt.ParityCount) {
		logger.Warn("has not enough providers")
	}

	lp := &lastProvider{
		bucketID: bucketID,
		dc:       int(bopt.DataCount),
		pc:       int(bopt.ParityCount),
		pros:     pros,
	}

	m.proMap[bucketID] = lp
}

func (m *OrderMgr) loadLastProviders(bucketID uint64) []uint64 {
	key := store.NewKey(pb.MetaType_OrderProsKey, m.localID, bucketID)
	val, err := m.ds.Get(key)
	if err != nil {
		return nil
	}

	res := make([]uint64, len(val)/8)
	for i := 0; i < len(val)/8; i++ {
		res[i] = binary.BigEndian.Uint64(val[8*i : 8*(i+1)])
	}

	return res
}

func (m *OrderMgr) saveLastProviders(bucketID uint64) error {
	lp, ok := m.proMap[bucketID]
	if !ok {
		return ErrNotFound
	}

	buf := make([]byte, 8*len(lp.pros))
	for i, pid := range lp.pros {
		binary.BigEndian.PutUint64(buf[8*i:8*(i+1)], pid)
	}

	key := store.NewKey(pb.MetaType_OrderProsKey, m.localID, bucketID)
	return m.ds.Put(key, buf)
}

func (m *OrderMgr) checkLastProvider() {
	// check pros number and connect
}

func (m *OrderMgr) dispatch() {
	// get last pros

	var seg types.Segs

	for {
		m.segLock.Lock()
		if len(m.segs) == 0 {
			m.segLock.Unlock()
			return
		}
		seg = m.segs[0]
		m.segs = m.segs[1:]
		m.segLock.Unlock()

		segID, _ := segment.NewSegmentID(m.fsID, seg.BucketID, 0, 0)
		lp := m.proMap[seg.BucketID]
		for i, pid := range lp.pros {
			or, ok := m.orders[pid]
			if !ok {
				continue
			}

			segID.SetChunkID(uint32(i))
			for s := seg.Start; s < seg.Start+seg.Length; s++ {
				segID.SetBucketID(s)
				or.addData(segID.Bytes())
			}
		}
	}
}

func (m *OrderMgr) AddSeg(seg types.Segs) {
	m.segLock.Lock()
	m.segs = append(m.segs, seg)
	m.segLock.Unlock()
}
