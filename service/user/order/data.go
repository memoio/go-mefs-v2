package order

import (
	"encoding/binary"
	"math"
	"sync"
	"time"

	"github.com/bits-and-blooms/bitset"
	"github.com/fxamacker/cbor/v2"
	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/lib/utils"
)

type lastProsPerBucket struct {
	bucketID uint64
	dc, pc   int
	pros     []uint64 // update and save to local
	deleted  []uint64
}

// todo: change pro when quotation price is too high

func (m *OrderMgr) RegisterBucket(bucketID, nextOpID uint64, bopt *pb.BucketOption) {
	logger.Info("register order for bucket: ", bucketID, nextOpID)

	m.loadUnfinishedSegJobs(bucketID, nextOpID)

	storedPros, delPros := m.loadLastProsPerBucket(bucketID)

	pros := make([]uint64, bopt.DataCount+bopt.ParityCount)
	for i := 0; i < int(bopt.DataCount+bopt.ParityCount); i++ {
		pros[i] = math.MaxUint64
	}

	if len(storedPros) == 0 {
		res := make(chan uint64, len(m.pros))
		var wg sync.WaitGroup
		for _, pid := range m.pros {
			wg.Add(1)
			go func(pid uint64) {
				defer wg.Done()
				err := m.connect(pid)
				if err == nil {
					res <- pid

				}
			}(pid)
		}
		wg.Wait()

		close(res)

		tmpPros := make([]uint64, 0, len(m.pros))
		for pid := range res {
			tmpPros = append(tmpPros, pid)
		}

		tmpPros = removeDup(tmpPros)

		for i, pid := range tmpPros {
			if i >= int(bopt.DataCount+bopt.ParityCount) {
				break
			}
			pros[i] = pid
		}
	} else {
		for i, pid := range storedPros {
			if i >= int(bopt.DataCount+bopt.ParityCount) {
				break
			}
			pros[i] = pid
		}
	}

	lp := &lastProsPerBucket{
		bucketID: bucketID,
		dc:       int(bopt.DataCount),
		pc:       int(bopt.ParityCount),
		pros:     pros,
		deleted:  delPros,
	}

	m.updateProsForBucket(lp)

	logger.Info("order bucket: ", lp.bucketID, lp.pros)

	m.bucketChan <- lp
}

func (m *OrderMgr) loadLastProsPerBucket(bucketID uint64) ([]uint64, []uint64) {
	key := store.NewKey(pb.MetaType_OrderProsKey, m.localID, bucketID)
	val, err := m.ds.Get(key)
	if err != nil {
		return nil, nil
	}

	res := make([]uint64, len(val)/8)
	for i := 0; i < len(val)/8; i++ {
		res[i] = binary.BigEndian.Uint64(val[8*i : 8*(i+1)])
	}

	key = store.NewKey(pb.MetaType_OrderProsDeleteKey, m.localID, bucketID)
	val, err = m.ds.Get(key)
	if err != nil {
		return res, nil
	}

	delres := make([]uint64, len(val)/8)
	for i := 0; i < len(val)/8; i++ {
		delres[i] = binary.BigEndian.Uint64(val[8*i : 8*(i+1)])
	}

	return res, delres
}

func (m *OrderMgr) saveLastProsPerBucket(lp *lastProsPerBucket) {
	buf := make([]byte, 8*len(lp.pros))
	for i, pid := range lp.pros {
		binary.BigEndian.PutUint64(buf[8*i:8*(i+1)], pid)
	}

	key := store.NewKey(pb.MetaType_OrderProsKey, m.localID, lp.bucketID)
	m.ds.Put(key, buf)

	buf = make([]byte, 8*len(lp.deleted))
	for i, pid := range lp.deleted {
		binary.BigEndian.PutUint64(buf[8*i:8*(i+1)], pid)
	}

	key = store.NewKey(pb.MetaType_OrderProsDeleteKey, m.localID, lp.bucketID)
	m.ds.Put(key, buf)
}

func removeDup(a []uint64) []uint64 {
	res := make([]uint64, 0, len(a))
	tMap := make(map[uint64]struct{}, len(a))
	for _, ai := range a {
		_, has := tMap[ai]
		if !has {
			tMap[ai] = struct{}{}
			res = append(res, ai)
		}
	}
	return res
}

func (m *OrderMgr) updateProsForBucket(lp *lastProsPerBucket) {
	cnt := 0
	for _, pid := range lp.pros {
		if pid == math.MaxUint64 {
			continue
		}
		or, ok := m.orders[pid]
		if ok {
			if !or.inStop {
				cnt++
			}
		}
	}

	if cnt >= lp.dc+lp.pc {
		return
	}

	otherPros := make([]uint64, 0, lp.dc+lp.pc)

	pros := make([]uint64, 0, len(m.pros))
	pros = append(pros, m.pros...)

	// todo:
	// 1. skip deleted first
	// 2. if not enough, add deleted?
	utils.DisorderUint(pros)

	for _, pid := range pros {
		has := false
		for _, hasPid := range lp.pros {
			if pid == hasPid {
				has = true
			}
		}

		if has {
			continue
		}
		or, ok := m.orders[pid]
		if ok {
			if or.ready && !or.inStop {
				otherPros = append(otherPros, pid)
			}
		}
	}

	change := false

	j := 0
	for i := 0; i < lp.dc+lp.pc; i++ {
		pid := lp.pros[i]
		if pid != math.MaxUint64 {
			or, ok := m.orders[pid]
			if ok {
				if !or.inStop {
					continue
				}
			}
		}

		for j < len(otherPros) {
			npid := otherPros[j]
			j++
			or, ok := m.orders[npid]
			if ok {
				if !or.inStop && m.ready {
					change = true
					lp.pros[i] = npid
					lp.deleted = append(lp.deleted, pid)
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

		seg := new(segJob)
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

		seg := new(segJob)
		key = store.NewKey(pb.MetaType_LFS_OpStateKey, m.localID, bucketID, i)
		data, _ = m.ds.Get(key)
		if len(data) > 0 {
			seg.Deserialize(data)
		}

		if seg.dispatchBits == nil {
			seg.dispatchBits = bitset.New(uint(sj.Length) * uint(sj.ChunkID))
			seg.doneBits = bitset.New(uint(sj.Length) * uint(sj.ChunkID))
		}

		if seg.doneBits.Count() == uint(sj.Length)*uint(sj.ChunkID) {
			buf := make([]byte, 8)
			binary.BigEndian.PutUint64(buf, sj.JobID)
			m.ds.Put(opckey, buf)
		} else {
			logger.Debug("load unfinished job:", bucketID, i, seg.doneBits.Count(), seg.dispatchBits.Count())

			seg.dispatchBits = bitset.From(seg.doneBits.Bytes())

			jk := jobKey{
				bucketID: bucketID,
				jobID:    i,
			}
			m.segLock.Lock()
			seg.SegJob = *sj
			m.segs[jk] = seg
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
		logger.Warn("fail to add seg:", seg.Start, seg.Length, sj.Start)
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
	if !ok {
		logger.Warn("fail finished seg: ", sj.JobID, sj.Start, sj.ChunkID)
		return
	}

	id := uint(sj.Start-seg.Start)*uint(seg.ChunkID) + uint(sj.ChunkID)
	if !seg.dispatchBits.Test(id) {
		logger.Warn("seg is not dispatch")
	}
	seg.doneBits.Set(id)

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
		jobkey := store.NewKey(pb.MetaType_LFS_OpJobsKey, m.localID, sj.BucketID, sj.JobID)
		val, err := m.ds.Get(jobkey)
		if err != nil {
			return
		}

		nsj := new(types.SegJob)
		err = cbor.Unmarshal(val, nsj)
		if err != nil {
			return
		}

		if seg.doneBits.Count() == uint(nsj.Length)*uint(nsj.ChunkID) {
			m.segLock.Lock()
			delete(m.segs, jk)
			m.segLock.Unlock()
		}
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
			if pid == math.MaxUint64 {
				logger.Debug("fail dispatch to wrong pro:", pid)
				continue
			}
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
				time.Sleep(time.Second)
				continue
			}

			if o.seq == nil || o.seqState != OrderSeq_Send {
				time.Sleep(time.Second)
				continue
			}

			if !o.hasSeg() {
				time.Sleep(time.Second)
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

			//logger.Debug("send segment:", sid.GetBucketID(), sid.GetStripeID(), sid.GetChunkID())

			o.availTime = time.Now().Unix()

			as := &types.AggSegs{
				BucketID: sid.GetBucketID(),
				Start:    sid.GetStripeID(),
				Length:   1,
				ChunkID:  sid.GetChunkID(),
			}

			o.Lock()

			// o.seq.DataName = append(o.seq.DataName, sid.Bytes())
			// update price and size
			o.seq.Segments.Push(as)
			o.seq.Price.Add(o.seq.Price, o.segPrice)
			o.seq.Size += build.DefaultSegSize

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
