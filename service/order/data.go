package order

import (
	"sync"

	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
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

	pros := make([]uint64, int(bopt.DataCount+bopt.ParityCount))
	proValue := m.proSet.Values()

	// sort

	for i, pid := range proValue {
		pros[i] = pid.(uint64)
	}

	lp := &lastProvider{
		bucketID: bucketID,
		dc:       int(bopt.DataCount),
		pc:       int(bopt.ParityCount),
		pros:     pros,
	}

	m.proMap[bucketID] = lp
}

func (m *OrderMgr) checkLastProvider() {
	// check pros number and connect
}

func (m *OrderMgr) addSeg(seg types.Segs) {
	// get last pros

	lp := m.proMap[seg.BucketID]
	for _, pid := range lp.pros {
		or, ok := m.orders[pid]
		if !ok {
			continue
		}

		or.addSeg(seg)
	}

	// add to order
}
