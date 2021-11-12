package order

import (
	"encoding/binary"
	"time"

	"github.com/bits-and-blooms/bitset"
	"github.com/fxamacker/cbor/v2"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/lib/utils"
)

type lastProsPerBucket struct {
	bucketID uint64
	dc, pc   int
	pros     []uint64 // pre pros; update and save to local
}

func (m *OrderMgr) RegisterBucket(bucketID uint64, bopt *pb.BucketOption) {
	_, ok := m.proMap[bucketID]
	if ok {
		return
	}

	storedPros := m.loadLastProsPerBucket(bucketID)

	pros := make([]uint64, int(bopt.DataCount+bopt.ParityCount))
	copy(pros, storedPros)

	lp := &lastProsPerBucket{
		bucketID: bucketID,
		dc:       int(bopt.DataCount),
		pc:       int(bopt.ParityCount),
		pros:     pros,
	}

	m.updateProsForBucket(lp)

	m.proMap[bucketID] = lp
}

func (m *OrderMgr) loadLastProsPerBucket(bucketID uint64) []uint64 {
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

func (m *OrderMgr) saveLastProsPerBucket(lp *lastProsPerBucket) error {
	buf := make([]byte, 8*len(lp.pros))
	for i, pid := range lp.pros {
		binary.BigEndian.PutUint64(buf[8*i:8*(i+1)], pid)
	}

	key := store.NewKey(pb.MetaType_OrderProsKey, m.localID, lp.bucketID)
	return m.ds.Put(key, buf)
}

func (m *OrderMgr) updateProsForBucket(lp *lastProsPerBucket) {
	otherPros := make([]uint64, 0, lp.dc+lp.pc)
	proValue := m.proSet.Values()
	utils.Disorder(proValue)
	for _, pid := range proValue {
		has := false
		for _, hasPid := range lp.pros {
			if pid == hasPid {
				has = true
			}
		}

		if !has {
			otherPros = append(otherPros, pid.(uint64))
		}
	}

	change := false

	for i, j := 0, 0; i < lp.dc+lp.pc && j < len(otherPros); i++ {
		if i < len(lp.pros) {
			pid := lp.pros[i]
			or, ok := m.orders[pid]
			if ok {
				if !or.inStop {
					continue
				}
			}
		}

		for ; j < len(otherPros); j++ {
			pid := otherPros[j]
			or, ok := m.orders[pid]
			if ok {
				if !or.inStop {
					change = true
					lp.pros[i] = pid
					break
				}
			}
		}
	}

	if change {
		m.saveLastProsPerBucket(lp)
	}

}

type segJob struct {
	types.SegJob
	dispatch *bitset.BitSet
	done     *bitset.BitSet
}

type segJobState struct {
	Sj       types.SegJob
	Dispatch []uint64
	Done     []uint64
}

func (sj *segJob) Serialize() ([]byte, error) {
	sjs := &segJobState{
		Sj:       sj.SegJob,
		Dispatch: sj.dispatch.Bytes(),
		Done:     sj.done.Bytes(),
	}

	return cbor.Marshal(sjs)
}

func (sj *segJob) Deserialize(b []byte) error {
	sjs := new(segJobState)

	err := cbor.Unmarshal(b, sjs)
	if err != nil {
		return err
	}

	sj.SegJob = sjs.Sj
	sj.dispatch = bitset.From(sjs.Dispatch)
	sj.done = bitset.From(sjs.Done)

	return nil
}

func (m *OrderMgr) AddSegJob(sj *types.SegJob) {
	m.segAddChan <- sj
}

func (m *OrderMgr) RedoSegJob(sj *types.SegJob) {
	m.segRedoChan <- sj
}

// disptach, done, total
func (m *OrderMgr) GetSegJogState(bucketID, opID uint64) (int, int, int) {
	donecnt := 0
	discnt := 0
	totalcnt := 0
	has := false

	m.segLock.RLock()
	for _, seg := range m.segs {
		if opID == seg.JobID && bucketID == seg.BucketID {
			donecnt = int(seg.done.Count())
			discnt = int(seg.dispatch.Count())
			totalcnt = int(seg.Length) * int(seg.ChunkID)
			has = true
			break
		}
	}
	m.segLock.RUnlock()

	if !has {
		donecnt = 1
		discnt = 1
		totalcnt = 1
	}

	return donecnt, discnt, totalcnt
}

func (m *OrderMgr) dispatch() {
	i := 0
	for {
		select {
		case <-m.ctx.Done():
			return
		case sj := <-m.segAddChan:
			m.segLock.RLock()
			for _, seg := range m.segs {
				if sj.JobID == seg.JobID && sj.BucketID == seg.BucketID {
					if seg.Start+seg.Length == sj.Start {
						seg.Length += sj.Length
					}
				}
			}
			m.segLock.RUnlock()

			s := &segJob{
				SegJob:   *sj,
				dispatch: bitset.New(uint(sj.Length) * uint(sj.ChunkID)),
				done:     bitset.New(uint(sj.Length) * uint(sj.ChunkID)),
			}

			m.segLock.Lock()
			m.segs = append(m.segs, s)
			m.segLock.Unlock()
		case sj := <-m.segRedoChan:
			m.segLock.RLock()
			for _, seg := range m.segs {
				if sj.JobID == seg.JobID && sj.BucketID == seg.BucketID {
					id := uint(sj.Start-seg.Start)*uint(seg.ChunkID) + uint(sj.ChunkID)
					seg.dispatch.Clear(id)
				}
			}
			m.segLock.RUnlock()
		default:
			m.segLock.RLock()
			if len(m.segs) == i {
				i = 0
			}
			if len(m.segs) == 0 {
				m.segLock.Unlock()
				time.Sleep(5 * time.Second)
				continue
			}
			seg := m.segs[i]
			m.segLock.RUnlock()

			cnt := uint(seg.Length) * uint(seg.ChunkID)
			if seg.dispatch.Count() == cnt {
				i++
				continue
			}

			lp := m.proMap[seg.BucketID]
			m.updateProsForBucket(lp)

			for i, pid := range lp.pros {
				or, ok := m.orders[pid]
				if !ok {
					continue
				}

				for s := uint64(0); s < seg.Length; i++ {
					id := uint(s)*uint(seg.ChunkID) + uint(i)
					if seg.dispatch.Test(id) {
						continue
					}

					sj := &types.SegJob{
						BucketID: seg.BucketID,
						JobID:    seg.JobID,
						Start:    seg.Start + s,
						Length:   1,
						ChunkID:  uint32(i),
					}

					err := or.addSeg(sj)
					if err == nil {
						seg.dispatch.Set(id)
					}
				}

				data, err := seg.Serialize()
				if err != nil {
					continue
				}

				key := store.NewKey(pb.MetaType_LFS_OpStateKey, m.localID, seg.JobID)

				m.ds.Put(key, data)
			}
			i++
		}
	}
}
