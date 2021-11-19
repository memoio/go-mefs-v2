package order

import (
	"encoding/binary"
	"math/big"
	"sync"
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

func (m *OrderMgr) RegisterBucket(bucketID, nextOpID uint64, bopt *pb.BucketOption) {
	logger.Info("register order for bucket: ", bucketID, nextOpID)

	go m.loadUnfinishedSegJobs(bucketID, nextOpID)

	storedPros := m.loadLastProsPerBucket(bucketID)

	pros := make([]uint64, 0, int(bopt.DataCount+bopt.ParityCount))

	var wg sync.WaitGroup
	for _, pid := range storedPros {
		wg.Add(1)
		go func(pid uint64) {
			defer wg.Done()
			err := m.connect(pid)
			if err == nil {
				pros = append(pros, pid)
			}
		}(pid)
	}

	wg.Wait()

	if len(pros) > int(bopt.DataCount+bopt.ParityCount) {
		pros = pros[:int(bopt.DataCount+bopt.ParityCount)]
	}

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
	for pid, por := range m.orders {
		if por.ready && !por.inStop {
			pros = append(pros, pid)
		}
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
				if !or.inStop && m.ready {
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

// disptach, done, total
func (m *OrderMgr) GetSegJogState(bucketID, opID uint64) (int, int, int) {
	donecnt := 1
	discnt := 1
	totalcnt := 1

	jk := jobKey{
		bucketID: bucketID,
		jobID:    opID,
	}

	m.segLock.RLock()
	seg, ok := m.segs[jk]
	m.segLock.RUnlock()
	if ok {
		discnt = int(seg.dispatchBits.Count())
		donecnt = int(seg.doneBits.Count())
		totalcnt = int(seg.Length) * int(seg.ChunkID)
	} else {
		sj := new(types.SegJob)
		key := store.NewKey(pb.MetaType_LFS_OpJobsKey, m.localID, bucketID, opID)
		data, err := m.ds.Get(key)
		if err != nil {
			return totalcnt, discnt, donecnt
		}

		err = cbor.Unmarshal(data, sj)
		if err != nil {
			return totalcnt, discnt, donecnt
		}

		seg := new(segJobState)
		key = store.NewKey(pb.MetaType_LFS_OpStateKey, m.localID, bucketID, opID)

		data, err = m.ds.Get(key)
		if err != nil {
			return totalcnt, discnt, donecnt
		}
		err = seg.Deserialize(data)
		if err != nil {
			return totalcnt, discnt, donecnt
		}
		discnt = int(seg.dispatchBits.Count())
		donecnt = int(seg.doneBits.Count())
		totalcnt = int(sj.Length) * int(sj.ChunkID)

	}
	logger.Debug("seg state: ", bucketID, opID, totalcnt, discnt, donecnt)

	return totalcnt, discnt, donecnt
}

func (m *OrderMgr) AddSegJob(sj *types.SegJob) {
	m.segAddChan <- sj
}

func (m *OrderMgr) loadUnfinishedSegJobs(bucketID, opID uint64) {
	for !m.ready {
		time.Sleep(time.Second)
	}

	time.Sleep(30 * time.Second)

	opckey := store.NewKey(pb.MetaType_LFS_OpCountKey, m.localID, bucketID)
	opDoneCount := uint64(0)
	val, err := m.ds.Get(opckey)
	if err == nil && len(val) >= 8 {
		opDoneCount = binary.BigEndian.Uint64(val)
	}

	logger.Debug("load unfinished job from: ", bucketID, opDoneCount, opID)

	for i := opDoneCount + 1; i < opID; i++ {
		key := store.NewKey(pb.MetaType_LFS_OpJobsKey, m.localID, bucketID, i)
		data, err := m.ds.Get(key)
		if err != nil {
			continue
		}
		sj := new(types.SegJob)
		err = cbor.Unmarshal(data, sj)
		if err != nil {
			continue
		}

		segState := new(segJobState)
		key = store.NewKey(pb.MetaType_LFS_OpStateKey, m.localID, bucketID, i)
		data, err = m.ds.Get(key)
		if err != nil {
			continue
		}

		err = segState.Deserialize(data)
		if err != nil {
			continue
		}
		if segState.doneBits.Count() == uint(sj.Length)*uint(sj.ChunkID) {
			buf := make([]byte, 8)
			binary.BigEndian.PutUint64(buf, sj.JobID)
			m.ds.Put(opckey, buf)
		} else {
			logger.Debug("load unfinished job:", bucketID, i, segState.doneBits.Count(), segState.dispatchBits.Count())

			segState.dispatchBits = bitset.From(segState.doneBits.Bytes())

			jk := jobKey{
				bucketID: bucketID,
				jobID:    i,
			}
			m.segLock.Lock()
			m.segs[jk] = &segJob{
				SegJob:      *sj,
				segJobState: *segState,
			}
			m.segLock.Unlock()
		}
	}
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
		if seg.dispatchBits == nil {
			seg = &segJob{
				SegJob: types.SegJob{
					JobID:    sj.JobID,
					BucketID: sj.BucketID,
					Start:    sj.Start,
					Length:   0,
					ChunkID:  sj.ChunkID,
				},
				segJobState: segJobState{
					dispatchBits: bitset.New(uint(sj.Length) * uint(sj.ChunkID)),
					doneBits:     bitset.New(uint(sj.Length) * uint(sj.ChunkID)),
				},
			}
		}
		m.segLock.Lock()
		m.segs[jk] = seg
		m.segLock.Unlock()
	}

	if seg.Start+seg.Length == sj.Start {
		seg.Length += sj.Length
	} else {
		logger.Warn("fail to add seg")
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
		if !seg.dispatchBits.Test(id) {
			logger.Warn("seg is not dispatch")
		}
		seg.doneBits.Set(id)
	}

	data, err := seg.Serialize()
	if err != nil {
		return
	}

	key := store.NewKey(pb.MetaType_LFS_OpStateKey, m.localID, seg.BucketID, seg.JobID)
	m.ds.Put(key, data)

	key = store.NewKey(pb.MetaType_LFS_OpCountKey, m.localID, seg.BucketID)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, seg.JobID)
	m.ds.Put(key, buf)

	if seg.doneBits.Count() == uint(seg.Length)*uint(seg.ChunkID) {
		// done all; remove from it
		m.segLock.Lock()
		delete(m.segs, jk)
		m.segLock.Unlock()
	}
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
		seg.dispatchBits.Clear(id)
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
		if seg.dispatchBits.Count() == cnt {
			//logger.Debug("seg is done for bucket:", seg.BucketID, seg.JobID)
			continue
		}

		lp, ok := m.proMap[seg.BucketID]
		if !ok {
			logger.Debug("fail dispatch for bucket:", seg.BucketID)
			continue
		}
		m.updateProsForBucket(lp)

		if len(lp.pros) == 0 {
			logger.Debug("fail get providers dispatch for bucket:", seg.BucketID, len(lp.pros))
			continue
		}

		for i, pid := range lp.pros {
			or, ok := m.orders[pid]
			if !ok {
				logger.Debug("fail dispatch to pro:", pid)
				continue
			}

			for s := uint64(0); s < seg.Length; s++ {
				id := uint(s)*uint(seg.ChunkID) + uint(i)
				if seg.dispatchBits.Test(id) {
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
					seg.dispatchBits.Set(id)
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

func (o *OrderFull) hasSeg() bool {
	has := false
	o.RLock()
	for _, bid := range o.buckets {
		bjob, ok := o.jobs[bid]
		if ok && len(bjob.jobs) > 0 {
			has = true
			break
		}
	}
	o.RUnlock()

	return has
}

func (o *OrderFull) segCount() int {
	cnt := 0
	o.RLock()
	for _, bid := range o.buckets {
		bjob, ok := o.jobs[bid]
		if ok {
			cnt += len(bjob.jobs)
		}
	}
	o.RUnlock()

	return cnt
}

func (o *OrderFull) addSeg(sj *types.SegJob) error {
	o.Lock()
	if o.inStop {
		o.Unlock()
		return ErrState
	}

	bjob, ok := o.jobs[sj.BucketID]
	if ok {
		bjob.jobs = append(bjob.jobs, sj)
	} else {
		bjob = &bucketJob{
			jobs: make([]*types.SegJob, 0, 64),
		}

		bjob.jobs = append(bjob.jobs, sj)
		o.jobs[sj.BucketID] = bjob
		o.buckets = append(o.buckets, sj.BucketID)
	}
	o.Unlock()

	return nil
}

func (o *OrderFull) sendData() {
	i := 0
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

			if !o.hasSeg() {
				continue
			}

			o.Lock()
			if i >= len(o.buckets) {
				i = 0
			}
			bid := o.buckets[i]

			bjob, ok := o.jobs[bid]
			if !ok || len(bjob.jobs) == 0 {
				i++
				o.Unlock()
				continue
			}
			sj := bjob.jobs[0]
			o.inflight = true
			o.Unlock()

			sid, err := segment.NewSegmentID(o.fsID, sj.BucketID, sj.Start, sj.ChunkID)
			if err != nil {
				o.Lock()
				i++
				o.inflight = false
				o.Unlock()
				continue
			}

			err = o.SendSegmentByID(o.ctx, sid, o.pro)
			if err != nil {
				o.Lock()
				o.inflight = false
				i++
				o.Unlock()
				continue
			}

			logger.Debug("send segment:", sid.GetBucketID(), sid.GetStripeID(), sid.GetChunkID())

			o.availTime = time.Now().Unix()

			as := &types.AggSegs{
				BucketID: sid.GetBucketID(),
				Start:    sid.GetStripeID(),
				Length:   1,
			}

			o.Lock()

			// o.seq.DataName = append(o.seq.DataName, sid.Bytes())
			// update price and size
			o.seq.Segments.Push(as)
			o.seq.Price.Add(o.seq.Price, big.NewInt(100))
			o.seq.Size += 1

			o.accPrice.Add(o.accPrice, big.NewInt(100))
			o.accSize += 1

			bjob = o.jobs[bid]
			bjob.jobs = bjob.jobs[1:]
			o.inflight = false

			data, err := o.seq.Serialize()
			if err != nil {
				o.Unlock()
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
