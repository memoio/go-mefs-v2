package order

import (
	"encoding/binary"
	"time"

	"github.com/bits-and-blooms/bitset"
	"github.com/fxamacker/cbor/v2"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
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
	logger.Info("register order for bucket: ", bucketID)
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

	m.bucketChan <- lp
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

func removeDup(a []uint64) []uint64 {
	if len(a) < 2 {
		return a
	}
	i := 0
	for j := 1; j < len(a); j++ {
		if a[i] != a[j] {
			i++
			a[i] = a[j]
		}
	}
	return a[:i+1]
}

func (m *OrderMgr) updateProsForBucket(lp *lastProsPerBucket) {
	otherPros := make([]uint64, 0, lp.dc+lp.pc)

	lp.pros = removeDup(lp.pros)

	pros := make([]uint64, 0, len(m.orders))
	for pid := range m.orders {
		pros = append(pros, pid)
	}

	utils.DisorderUint(pros)
	for _, pid := range pros {
		has := false
		for _, hasPid := range lp.pros {
			if pid == hasPid {
				has = true
			}
		}

		if !has {
			otherPros = append(otherPros, pid)
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

		for j < len(otherPros) {
			pid := otherPros[j]
			j++
			or, ok := m.orders[pid]
			if ok {
				if !or.inStop {
					change = true
					if i < len(lp.pros) {
						lp.pros[i] = pid
					} else {
						lp.pros = append(lp.pros, pid)
					}

					break
				}
			}
		}
	}

	if change {
		m.saveLastProsPerBucket(lp)
	}
	logger.Info("order bucket: ", lp.bucketID, lp.pros)
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

// disptach, done, total
func (m *OrderMgr) GetSegJogState(bucketID, opID uint64) (int, int, int) {
	donecnt := 0
	discnt := 0
	totalcnt := 0

	jk := jobKey{
		bucketID: bucketID,
		jobID:    opID,
	}

	m.segLock.RLock()
	seg, ok := m.segs[jk]
	m.segLock.RUnlock()
	if ok {
		donecnt = int(seg.done.Count())
		discnt = int(seg.dispatch.Count())
		totalcnt = int(seg.Length) * int(seg.ChunkID)
	} else {
		donecnt = 1
		discnt = 1
		totalcnt = 1
	}

	return donecnt, discnt, totalcnt
}

func (m *OrderMgr) addSegJob(sj *types.SegJob) {
	jk := jobKey{
		bucketID: sj.BucketID,
		jobID:    sj.JobID,
	}

	key := store.NewKey(pb.MetaType_LFS_OpStateKey, m.localID, sj.BucketID, sj.JobID)

	m.segLock.RLock()
	seg, ok := m.segs[jk]
	m.segLock.RUnlock()
	if !ok {
		seg = new(segJob)
		// get from local first
		data, err := m.ds.Get(key)
		if err == nil && len(data) > 0 {
			seg.Deserialize(data)
		}
		if seg.dispatch == nil {
			seg = &segJob{
				SegJob:   *sj,
				dispatch: bitset.New(uint(sj.Length) * uint(sj.ChunkID)),
				done:     bitset.New(uint(sj.Length) * uint(sj.ChunkID)),
			}
		}
		m.segLock.Lock()
		m.segs[jk] = seg
		m.segLock.Unlock()
	}

	if seg.Start+seg.Length == sj.Start {
		seg.Length += sj.Length
	}

	data, err := seg.Serialize()
	if err != nil {
		return
	}

	m.ds.Put(key, data)
}

func (m *OrderMgr) finishSegJob(sj *types.SegJob) {
	jk := jobKey{
		bucketID: sj.BucketID,
		jobID:    sj.JobID,
	}

	m.segLock.RLock()
	seg, ok := m.segs[jk]
	m.segLock.RUnlock()
	if ok {
		id := uint(sj.Start-seg.Start)*uint(seg.ChunkID) + uint(sj.ChunkID)
		seg.done.Set(id)
	}

	data, err := seg.Serialize()
	if err != nil {
		return
	}

	key := store.NewKey(pb.MetaType_LFS_OpStateKey, m.localID, seg.BucketID, seg.JobID)

	m.ds.Put(key, data)
}

func (m *OrderMgr) redoSegJob(sj *types.SegJob) {
	jk := jobKey{
		bucketID: sj.BucketID,
		jobID:    sj.JobID,
	}
	m.segLock.RLock()
	seg, ok := m.segs[jk]
	m.segLock.RUnlock()
	if ok {
		id := uint(sj.Start-seg.Start)*uint(seg.ChunkID) + uint(sj.ChunkID)
		seg.dispatch.Clear(id)
	}

	data, err := seg.Serialize()
	if err != nil {
		return
	}

	key := store.NewKey(pb.MetaType_LFS_OpStateKey, m.localID, seg.BucketID, seg.JobID)

	m.ds.Put(key, data)
}

func (m *OrderMgr) dispatch() {
	m.segLock.RLock()
	defer m.segLock.RUnlock()

	for _, seg := range m.segs {
		cnt := uint(seg.Length) * uint(seg.ChunkID)
		if seg.dispatch.Count() == cnt {
			//logger.Debug("seg is done for bucket:", seg.BucketID, seg.JobID)
			continue
		}

		lp, ok := m.proMap[seg.BucketID]
		if !ok {
			logger.Debug("fail dispatch for bucket:", seg.BucketID)
			continue
		}
		m.updateProsForBucket(lp)

		for i, pid := range lp.pros {
			or, ok := m.orders[pid]
			if !ok {
				logger.Debug("fail dispatch to pro:", pid)
				continue
			}

			for s := uint64(0); s < seg.Length; s++ {
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

			key := store.NewKey(pb.MetaType_LFS_OpStateKey, m.localID, seg.BucketID, seg.JobID)

			m.ds.Put(key, data)
		}
	}
}

func (o *OrderFull) addSeg(sj *types.SegJob) error {
	o.RLock()
	if o.inStop {
		o.RUnlock()
		return ErrState
	}
	o.segs = append(o.segs, sj)
	o.RUnlock()

	return nil
}

func (o *OrderFull) sendData() {
	for {
		select {
		case <-o.ctx.Done():
			return
		default:
			if o.base == nil || o.orderState != Order_Running {
				continue
			}

			if o.seq == nil || o.seqState != OrderSeq_Send {
				continue
			}

			o.Lock()
			if len(o.segs) == 0 {
				o.Unlock()
				time.Sleep(10 * time.Second)
				continue
			}
			sj := o.segs[0]
			o.inflight = true
			o.Unlock()

			sid, err := segment.NewSegmentID(o.fsID, sj.BucketID, sj.Start, sj.ChunkID)
			if err != nil {
				o.Lock()
				o.inflight = false
				o.Unlock()
				continue
			}
			err = o.SendSegmentByID(o.ctx, sid, o.pro)
			if err != nil {
				o.Lock()
				o.inflight = false
				o.Unlock()
				continue
			}

			o.availTime = time.Now().Unix()

			o.Lock()
			o.seq.DataName = append(o.seq.DataName, sid.Bytes())
			// update price and size
			o.segs = o.segs[1:]
			o.inflight = false

			data, err := o.seq.Serialize()
			if err != nil {
				continue
			}
			o.Unlock()

			key := store.NewKey(pb.MetaType_OrderSeqKey, o.localID, o.pro, o.base.Nonce, o.seq.SeqNum)
			err = o.ds.Put(key, data)
			if err != nil {
				continue
			}

			o.segDoneChan <- sj
		}
	}
}
