package order

import (
	"encoding/binary"
	"math"
	"sync"
	"time"

	"github.com/bits-and-blooms/bitset"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/lib/utils"
)

type lastProsPerBucket struct {
	lk       sync.RWMutex
	bucketID uint64
	dc, pc   int
	pros     []uint64 // update and save to local
	deleted  []uint64 // add del pro here
}

// todo: change pro when quotation price is too high
// todo: fix order duplicated due to pro change

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

	m.saveLastProsPerBucket(lp)

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
		pid := binary.BigEndian.Uint64(val[8*i : 8*(i+1)])
		if pid != math.MaxUint64 {
			go m.newProOrder(pid)
		}

		res[i] = pid
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
	lp.lk.Lock()
	defer lp.lk.Unlock()

	cnt := 0
	for _, pid := range lp.pros {
		if pid == math.MaxUint64 {
			continue
		}
		m.lk.RLock()
		or, ok := m.orders[pid]
		m.lk.RUnlock()
		if ok {
			if !or.inStop {
				cnt++
			}
		}
	}

	if cnt >= lp.dc+lp.pc {
		return
	}

	logger.Debugf("order bucket %d expected %d, got %d", lp.bucketID, lp.dc+lp.pc, cnt)

	// expand providers

	otherPros := make([]uint64, 0, len(m.pros))
	for _, pid := range m.pros {
		has := false
		for _, dpid := range lp.deleted {
			if pid == dpid {
				has = true
				break
			}
		}

		if has {
			continue
		}

		// not deleted
		for _, hasPid := range lp.pros {
			if pid == hasPid {
				has = true
				break
			}
		}

		if has {
			continue
		}

		// not have
		m.lk.RLock()
		or, ok := m.orders[pid]
		m.lk.RUnlock()
		if ok {
			if or.ready && !or.inStop {
				otherPros = append(otherPros, pid)
			}
		}
	}

	// todo:
	// 1. skip deleted first
	// 2. if not enough, add deleted?
	utils.DisorderUint(otherPros)

	change := false

	j := 0
	for i := 0; i < lp.dc+lp.pc; i++ {
		pid := lp.pros[i]
		if pid != math.MaxUint64 {
			m.lk.RLock()
			or, ok := m.orders[pid]
			m.lk.RUnlock()
			if ok {
				if !or.inStop {
					continue
				}
			}
		}

		for j < len(otherPros) {
			npid := otherPros[j]
			j++
			m.lk.RLock()
			or, ok := m.orders[npid]
			m.lk.RUnlock()
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
	logger.Info("order bucket: ", lp.bucketID, lp.pros, lp.deleted)
}

// disptach, done, total
func (m *OrderMgr) getSegJob(bucketID, opID uint64, all bool, add bool) (*segJob, uint, error) {
	jk := jobKey{
		bucketID: bucketID,
		jobID:    opID,
	}

	cnt := uint(0)

	m.segLock.RLock()
	seg, ok := m.segs[jk]
	m.segLock.RUnlock()
	if !ok {
		sj := new(types.SegJob)
		key := store.NewKey(pb.MetaType_LFS_OpJobsKey, m.localID, bucketID, opID)
		data, err := m.ds.Get(key)
		if err != nil {
			return nil, cnt, err
		}

		err = sj.Deserialize(data)
		if err != nil {
			return nil, cnt, err
		}

		cnt = uint(sj.Length) * uint(sj.ChunkID)

		seg = new(segJob)
		key = store.NewKey(pb.MetaType_LFS_OpStateKey, m.localID, bucketID, opID)
		data, err = m.ds.Get(key)
		if err == nil && len(data) > 0 {
			seg.Deserialize(data)
		}
		if seg.dispatchBits == nil {
			seg = &segJob{
				SegJob: types.SegJob{
					BucketID: sj.BucketID,
					JobID:    sj.JobID,
					Start:    sj.Start,
					Length:   0, // add later
					ChunkID:  sj.ChunkID,
				},
				segJobState: segJobState{
					dispatchBits: bitset.New(cnt),
					doneBits:     bitset.New(cnt),
					confirmBits:  bitset.New(cnt),
				},
			}

			if all {
				seg.SegJob = *sj
			}
		}

		if add && cnt > 0 && seg.confirmBits.Count() != cnt {
			m.segLock.Lock()
			m.segs[jk] = seg
			m.segLock.Unlock()
		}

		logger.Debug("load seg: ", bucketID, opID, cnt, seg.dispatchBits.Count(), seg.doneBits.Count(), seg.confirmBits.Count())
	}

	return seg, cnt, nil
}

func (m *OrderMgr) GetSegJogState(bucketID, opID uint64) (int, int, int, int) {
	confirmCnt := 1
	donecnt := 1
	discnt := 1
	totalcnt := 1

	sj := new(types.SegJob)
	key := store.NewKey(pb.MetaType_LFS_OpJobsKey, m.localID, bucketID, opID)
	data, err := m.ds.Get(key)
	if err != nil {
		return totalcnt, discnt, donecnt, confirmCnt
	}

	err = sj.Deserialize(data)
	if err != nil {
		return totalcnt, discnt, donecnt, confirmCnt
	}

	cnt := uint(sj.Length) * uint(sj.ChunkID)

	seg, _, err := m.getSegJob(bucketID, opID, false, false)
	if err != nil {
		return totalcnt, discnt, donecnt, confirmCnt
	}

	discnt = int(seg.dispatchBits.Count())
	donecnt = int(seg.doneBits.Count())
	confirmCnt = int(seg.confirmBits.Count())
	totalcnt = int(cnt)

	logger.Debug("seg state: ", bucketID, opID, totalcnt, discnt, donecnt, confirmCnt)

	return totalcnt, discnt, donecnt, confirmCnt
}

func (m *OrderMgr) AddSegJob(sj *types.SegJob) {
	m.segAddChan <- sj
}

func (m *OrderMgr) loadUnfinishedSegJobs(bucketID, opID uint64) {
	for !m.ready {
		time.Sleep(time.Second)
	}

	time.Sleep(30 * time.Second)

	// todo
	opckey := store.NewKey(pb.MetaType_LFS_OpCountKey, m.localID, bucketID)
	opDoneCount := uint64(0)
	val, err := m.ds.Get(opckey)
	if err == nil && len(val) >= 8 {
		opDoneCount = binary.BigEndian.Uint64(val)
	}

	logger.Debug("load unfinished job from: ", bucketID, opDoneCount, opID)

	for i := opDoneCount + 1; i < opID; i++ {
		seg, cnt, err := m.getSegJob(bucketID, i, true, true)
		if err != nil {
			continue
		}

		if seg.confirmBits.Count() != cnt {
			seg.dispatchBits = bitset.From(seg.doneBits.Bytes())
			logger.Debug("load unfinished job:", bucketID, i, seg.dispatchBits.Count(), seg.doneBits.Count(), seg.confirmBits.Count(), cnt)
		}
	}
}

func (m *OrderMgr) addSegJob(sj *types.SegJob) {
	logger.Debug("add seg: ", sj.BucketID, sj.JobID, sj.Start, sj.Length, sj.ChunkID)

	seg, _, err := m.getSegJob(sj.BucketID, sj.JobID, false, true)
	if err != nil {
		logger.Debug("fail to add seg:", sj.BucketID, sj.JobID, seg.Start, seg.Length, sj.Start, err)
		return
	}

	if seg.Start+seg.Length == sj.Start {
		seg.Length += sj.Length
	} else {
		logger.Debug("fail to add seg:", sj.BucketID, sj.JobID, seg.Start, seg.Length, sj.Start)
		return
	}

	key := store.NewKey(pb.MetaType_LFS_OpStateKey, m.localID, sj.BucketID, sj.JobID)
	data, err := seg.Serialize()
	if err != nil {
		return
	}

	m.ds.Put(key, data)
}

func (m *OrderMgr) finishSegJob(sj *types.SegJob) {
	logger.Debug("finish seg: ", sj.BucketID, sj.JobID, sj.Start, sj.Length, sj.ChunkID)

	seg, _, err := m.getSegJob(sj.BucketID, sj.JobID, true, true)
	if err != nil {
		logger.Debug("fail to finish seg:", seg.Start, seg.Length, sj.Start, err)
		return
	}

	for i := uint64(0); i < sj.Length; i++ {
		id := uint(sj.Start+i-seg.Start)*uint(seg.ChunkID) + uint(sj.ChunkID)
		if !seg.dispatchBits.Test(id) {
			logger.Debug("finish seg is not dispatch: ", sj.BucketID, sj.JobID, sj.Start, sj.Length, sj.ChunkID)
		}
		seg.doneBits.Set(id)
	}

	data, err := seg.Serialize()
	if err != nil {
		return
	}

	key := store.NewKey(pb.MetaType_LFS_OpStateKey, m.localID, seg.BucketID, seg.JobID)
	m.ds.Put(key, data)
}

func (m *OrderMgr) confirmSegJob(sj *types.SegJob) {
	logger.Debug("confirm seg: ", sj.BucketID, sj.JobID, sj.Start, sj.Length, sj.ChunkID)

	seg, _, err := m.getSegJob(sj.BucketID, sj.JobID, true, true)
	if err != nil {
		logger.Debug("fail to confirm seg:", seg.Start, seg.Length, sj.Start, err)
		return
	}

	for i := uint64(0); i < sj.Length; i++ {
		id := uint(sj.Start+i-seg.Start)*uint(seg.ChunkID) + uint(sj.ChunkID)
		if !seg.dispatchBits.Test(id) {
			logger.Debug("confirm seg is not dispatch in confirm: ", sj.BucketID, sj.JobID, sj.Start, i, sj.ChunkID)
		}

		if !seg.doneBits.Test(id) {
			logger.Debug("confirm seg is not sent in confirm: ", sj.BucketID, sj.JobID, sj.Start, i, sj.ChunkID)
			// reset again
			seg.doneBits.Set(id)
		}
		seg.confirmBits.Set(id)
	}

	data, err := seg.Serialize()
	if err != nil {
		return
	}

	key := store.NewKey(pb.MetaType_LFS_OpStateKey, m.localID, seg.BucketID, seg.JobID)
	m.ds.Put(key, data)

	cnt := uint(seg.Length) * uint(seg.ChunkID)
	if seg.doneBits.Count() == cnt && seg.confirmBits.Count() == cnt {
		// reget for sure
		jobkey := store.NewKey(pb.MetaType_LFS_OpJobsKey, m.localID, sj.BucketID, sj.JobID)
		val, err := m.ds.Get(jobkey)
		if err != nil {
			return
		}

		nsj := new(types.SegJob)
		err = nsj.Deserialize(val)
		if err != nil {
			return
		}

		// done and confirm all; remove from memory
		if uint(nsj.Length)*uint(nsj.ChunkID) == cnt {
			logger.Debug("confirm seg: ", sj.BucketID, sj.JobID, sj.Start, sj.Length, sj.ChunkID, cnt)
			jk := jobKey{
				bucketID: sj.BucketID,
				jobID:    sj.JobID,
			}
			m.segLock.Lock()
			delete(m.segs, jk)
			m.segLock.Unlock()
			key = store.NewKey(pb.MetaType_LFS_OpCountKey, m.localID, seg.BucketID)
			buf := make([]byte, 8)
			binary.BigEndian.PutUint64(buf, seg.JobID)
			m.ds.Put(key, buf)
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
		for i := uint64(0); i < sj.Length; i++ {
			id := uint(sj.Start+i-seg.Start)*uint(seg.ChunkID) + uint(sj.ChunkID)
			seg.dispatchBits.Clear(id)
			seg.doneBits.Clear(id)
		}

		data, err := seg.Serialize()
		if err != nil {
			return
		}

		key := store.NewKey(pb.MetaType_LFS_OpStateKey, m.localID, seg.BucketID, seg.JobID)
		m.ds.Put(key, data)
		return
	}

	logger.Debug("fail redo job: ", sj)
}

func (m *OrderMgr) dispatch() {
	m.segLock.RLock()
	defer m.segLock.RUnlock()

	for _, seg := range m.segs {
		cnt := uint(seg.Length) * uint(seg.ChunkID)
		if seg.dispatchBits.Count() == cnt {
			logger.Debug("seg is dispatched: ", seg.BucketID, seg.JobID, cnt)
			continue
		}

		logger.Debug("seg is disptch: ", seg.BucketID, seg.JobID)

		lp, ok := m.proMap[seg.BucketID]
		if !ok {
			logger.Debug("fail dispatch for bucket: ", seg.BucketID)
			continue
		}
		m.updateProsForBucket(lp)

		lp.lk.RLock()
		pros := make([]uint64, 0, len(lp.pros))
		pros = append(pros, lp.pros...)
		lp.lk.RUnlock()

		if len(pros) == 0 {
			logger.Debug("fail get providers dispatch for bucket:", seg.BucketID, len(pros))
			continue
		}

		for i, pid := range pros {
			if pid == math.MaxUint64 {
				logger.Debug("fail dispatch to wrong pro:", pid)
				continue
			}
			m.lk.RLock()
			or, ok := m.orders[pid]
			m.lk.RUnlock()
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
	defer o.Unlock()

	if o.inStop {
		return xerrors.Errorf("%d is stop", o.pro)
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

	return nil
}

// todo: add parallel control here; goprocess
func (m *OrderMgr) sendData(o *OrderFull) {
	i := 0
	for {
		select {
		case <-m.ctx.Done():
			return
		default:
			if o.inStop {
				time.Sleep(time.Minute)
				continue
			}

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

			err := m.sendCtr.Acquire(m.ctx, 1)
			if err != nil {
				time.Sleep(time.Second)
				continue
			}

			o.Lock()
			// should i++ here?
			i++
			if i >= len(o.buckets) {
				i = 0
			}
			bid := o.buckets[i]

			bjob, ok := o.jobs[bid]
			if !ok || len(bjob.jobs) == 0 {
				o.Unlock()
				m.sendCtr.Release(1)
				continue
			}
			sj := bjob.jobs[0]
			o.inflight = true
			o.Unlock()

			sid, err := segment.NewSegmentID(o.fsID, sj.BucketID, sj.Start, sj.ChunkID)
			if err != nil {
				m.sendCtr.Release(1)
				o.Lock()
				o.inflight = false
				o.Unlock()
				continue
			}

			err = o.SendSegmentByID(o.ctx, sid, o.pro)
			if err != nil {
				m.sendCtr.Release(1)
				logger.Debug("send segment fail:", o.pro, sid.GetBucketID(), sid.GetStripeID(), sid.GetChunkID(), err)

				// weather has been sent
				key := store.NewKey(pb.MetaType_SegLocationKey, sid.ToString())
				has, err := m.ds.Has(key)
				if err != nil || !has {
					o.Lock()
					o.inflight = false
					o.Unlock()
				} else {
					// has sent
					o.Lock()
					bjob = o.jobs[bid]
					bjob.jobs = bjob.jobs[1:]
					o.inflight = false
					o.Unlock()
					m.segDoneChan <- sj
				}

				continue
			}
			m.sendCtr.Release(1)

			logger.Debug("send segment:", o.pro, sid.GetBucketID(), sid.GetStripeID(), sid.GetChunkID())

			o.availTime = time.Now().Unix()

			as := &types.AggSegs{
				BucketID: sid.GetBucketID(),
				Start:    sid.GetStripeID(),
				Length:   1,
				ChunkID:  sid.GetChunkID(),
			}

			o.Lock()

			// update price an size
			o.seq.Segments.Push(as)
			o.seq.Segments.Merge()
			o.seq.Price.Add(o.seq.Price, o.base.SegPrice)
			o.seq.Size += build.DefaultSegSize

			nsj := &types.SegJob{
				BucketID: sj.BucketID,
				JobID:    sj.JobID,
				Start:    sj.Start,
				Length:   sj.Length,
				ChunkID:  sj.ChunkID,
			}

			o.sjq.Push(nsj)
			o.sjq.Merge()

			bjob = o.jobs[bid]
			bjob.jobs = bjob.jobs[1:]
			o.inflight = false

			saveOrderSeq(o, m.ds)
			saveSeqJob(o, m.ds)

			o.Unlock()

			m.segDoneChan <- sj
		}
	}
}
