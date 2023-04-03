package order

import (
	"encoding/binary"
	"math"
	"os"
	"strings"
	"time"

	"github.com/bits-and-blooms/bitset"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

// jobs
// add segjob
func (m *OrderMgr) runSegSched() {
	st := time.NewTicker(10 * time.Second)
	defer st.Stop()

	for {
		// handle data
		select {
		case <-m.ctx.Done():
			return
		case sj := <-m.segAddChan:
			m.addSegJob(sj)
		case sj := <-m.segRedoChan:
			m.redoSegJob(sj)
		case sj := <-m.segSentChan:
			m.finishSegJob(sj)
		case sj := <-m.segConfirmChan:
			m.confirmSegJob(sj)
		case <-st.C:
			// dispatch to each pro
			m.dispatch()
		}
	}
}

// external api
// get job state
func (m *OrderMgr) GetSegJogState(bucketID, opID uint64) (totalcnt, discnt, donecnt, confirmCnt int) {
	seg, err := m.loadSegJob(bucketID, opID)
	if err != nil {
		return totalcnt, discnt, donecnt, confirmCnt
	}

	discnt = int(seg.dispatchBits.Count())
	donecnt = int(seg.doneBits.Count())
	confirmCnt = int(seg.confirmBits.Count())
	totalcnt = int(seg.Length) * int(seg.ChunkID)

	logger.Debug("seg state: ", bucketID, opID, totalcnt, discnt, donecnt, confirmCnt)

	return totalcnt, discnt, donecnt, confirmCnt
}

// add seg job to order service
func (m *OrderMgr) AddSegJob(sj *types.SegJob) {
	m.segAddChan <- sj
}

func (m *OrderMgr) loadUnfinishedSegJobs(bucketID, opID uint64) {
	for !m.ready {
		time.Sleep(time.Second)
	}

	// todo: why?
	time.Sleep(30 * time.Second)

	opckey := store.NewKey(pb.MetaType_LFS_OpCountKey, m.localID, bucketID)
	opDoneCount := uint64(0)
	val, err := m.ds.Get(opckey)
	if err == nil && len(val) >= 8 {
		opDoneCount = binary.BigEndian.Uint64(val) + 1
	}

	if os.Getenv("MEFS_RECOVERY_MODE") != "" {
		opDoneCount = 0
	}

	logger.Info("load unfinished job: ", bucketID, opDoneCount, opID)

	for i := opDoneCount; i < opID; i++ {
		seg, err := m.loadSegJob(bucketID, i)
		if err != nil {
			continue
		}

		if seg.confirmBits.Count() != uint(seg.Length)*uint(seg.ChunkID) {
			m.segRedoChan <- &seg.SegJob
			logger.Info("load unfinished job: ", bucketID, i, seg.dispatchBits.Count(), seg.doneBits.Count(), seg.confirmBits.Count(), uint(seg.Length)*uint(seg.ChunkID))
		}
	}
}

func (m *OrderMgr) loadSegJob(bucketID, opID uint64) (*segJob, error) {
	sj := new(types.SegJob)
	key := store.NewKey(pb.MetaType_LFS_OpJobsKey, m.localID, bucketID, opID)
	data, err := m.ds.Get(key)
	if err != nil {
		return nil, err
	}

	err = sj.Deserialize(data)
	if err != nil {
		return nil, err
	}

	cnt := uint(sj.Length) * uint(sj.ChunkID)

	seg := new(segJob)
	key = store.NewKey(pb.MetaType_LFS_OpStateKey, m.localID, bucketID, opID)
	data, err = m.ds.Get(key)
	if err == nil && len(data) > 0 {
		seg.Deserialize(data)
	}
	if seg.dispatchBits == nil {
		seg = &segJob{
			SegJob: *sj,
			segJobState: segJobState{
				dispatchBits: bitset.New(cnt),
				doneBits:     bitset.New(cnt),
				confirmBits:  bitset.New(cnt),
			},
		}
	} else {
		if seg.confirmBits == nil {
			seg.confirmBits = bitset.New(cnt)
		}

		seg.dispatchBits = seg.doneBits.Clone()
	}

	logger.Debug("load seg: ", bucketID, opID, cnt, seg.dispatchBits.Count(), seg.doneBits.Count(), seg.confirmBits.Count())

	return seg, nil
}

func (m *OrderMgr) addSegJob(sj *types.SegJob) {
	logger.Debug("add seg: ", sj.BucketID, sj.JobID, sj.Start, sj.Length, sj.ChunkID)

	jk := jobKey{
		bucketID: sj.BucketID,
		jobID:    sj.JobID,
	}

	_, ok := m.segs[jk]
	if !ok {
		seg, err := m.loadSegJob(sj.BucketID, sj.JobID)
		if err != nil {
			logger.Debug("fail to load seg: ", sj.BucketID, sj.JobID, seg.Start, seg.Length, sj.Start, err)
			return
		}

		m.segs[jk] = seg

		key := store.NewKey(pb.MetaType_LFS_OpStateKey, m.localID, sj.BucketID, sj.JobID)
		data, err := seg.Serialize()
		if err != nil {
			return
		}

		m.ds.Put(key, data)
	}
}

func (m *OrderMgr) finishSegJob(sj *types.SegJob) {
	logger.Debug("sent seg: ", sj.BucketID, sj.JobID, sj.Start, sj.Length, sj.ChunkID)

	jk := jobKey{
		bucketID: sj.BucketID,
		jobID:    sj.JobID,
	}

	seg, ok := m.segs[jk]
	if !ok {
		nseg, err := m.loadSegJob(sj.BucketID, sj.JobID)
		if err != nil {
			logger.Debug("fail to load seg: ", sj.BucketID, sj.JobID, seg.Start, seg.Length, sj.Start, err)
			return
		}

		m.segs[jk] = nseg
		seg = nseg
	}

	for i := uint64(0); i < sj.Length; i++ {
		id := uint(sj.Start+i-seg.Start)*uint(seg.ChunkID) + uint(sj.ChunkID)
		if !seg.dispatchBits.Test(id) {
			logger.Debug("sent seg is not dispatch: ", sj.BucketID, sj.JobID, sj.Start, sj.Length, sj.ChunkID)
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

	jk := jobKey{
		bucketID: sj.BucketID,
		jobID:    sj.JobID,
	}

	seg, ok := m.segs[jk]
	if !ok {
		nseg, err := m.loadSegJob(sj.BucketID, sj.JobID)
		if err != nil {
			logger.Debug("fail to load seg: ", sj.BucketID, sj.JobID, seg.Start, seg.Length, sj.Start, err)
			return
		}

		m.segs[jk] = nseg
		seg = nseg
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
			delete(m.segs, jk)
			key = store.NewKey(pb.MetaType_LFS_OpCountKey, m.localID, seg.BucketID)
			buf := make([]byte, 8)
			binary.BigEndian.PutUint64(buf, seg.JobID)
			m.ds.Put(key, buf)
		}
	}
}

func (m *OrderMgr) redoSegJob(sj *types.SegJob) {
	logger.Debug("redo seg: ", sj.BucketID, sj.JobID, sj.Start, sj.Length, sj.ChunkID)
	jk := jobKey{
		bucketID: sj.BucketID,
		jobID:    sj.JobID,
	}

	seg, ok := m.segs[jk]
	if !ok {
		nseg, err := m.loadSegJob(sj.BucketID, sj.JobID)
		if err != nil {
			logger.Debug("fail to load seg: ", sj.BucketID, sj.JobID, seg.Start, seg.Length, sj.Start, err)
			return
		}

		m.segs[jk] = nseg
		seg = nseg
	}

	for i := uint64(0); i < sj.Length; i++ {
		id := uint(sj.Start+i-seg.Start)*uint(seg.ChunkID) + uint(sj.ChunkID)
		if !seg.confirmBits.Test(id) {
			seg.dispatchBits.Clear(id)
			seg.doneBits.Clear(id)
		}
	}

	data, err := seg.Serialize()
	if err != nil {
		return
	}

	key := store.NewKey(pb.MetaType_LFS_OpStateKey, m.localID, seg.BucketID, seg.JobID)
	m.ds.Put(key, data)
}

func (m *OrderMgr) dispatch() {
	logger.Debug("dispatch start")

	prom := make(map[uint64][]uint64)

	nt := time.Now()
	dcnt := 0
	for _, seg := range m.segs {
		if time.Since(nt).Seconds() > 8 {
			break
		}
		cnt := uint(seg.Length) * uint(seg.ChunkID)
		if seg.dispatchBits.Count() == cnt {
			logger.Debug("seg is dispatched: ", seg.BucketID, seg.JobID, cnt)
			continue
		}

		logger.Debug("seg is disptch: ", seg.BucketID, seg.JobID)

		pros, ok := prom[seg.BucketID]
		if !ok {
			m.lk.RLock()
			lp, ok := m.bucMap[seg.BucketID]
			m.lk.RUnlock()
			if !ok {
				logger.Debug("fail dispatch for bucket: ", seg.BucketID)
				continue
			}

			lp.lk.RLock()
			pros = make([]uint64, 0, len(lp.pros))
			pros = append(pros, lp.pros...)
			lp.lk.RUnlock()

			if len(pros) == 0 {
				logger.Debug("fail get providers dispatch for bucket: ", seg.BucketID, len(pros))
				continue
			}

			prom[seg.BucketID] = pros
		}

		dcnt++

		for i, pid := range pros {
			if pid == math.MaxUint64 {
				logger.Debug("fail dispatch to wrong pro: ", pid)
				continue
			}
			m.lk.RLock()
			or, ok := m.pInstMap[pid]
			m.lk.RUnlock()
			if !ok {
				logger.Debug("fail dispatch to pro: ", pid)
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
		}
		data, err := seg.Serialize()
		if err != nil {
			continue
		}
		key := store.NewKey(pb.MetaType_LFS_OpStateKey, m.localID, seg.BucketID, seg.JobID)
		m.ds.Put(key, data)
	}
	logger.Debug("dispatch cost: ", dcnt, time.Since(nt))
}

// dispatch seg chunk to provider
func (o *proInst) addSeg(sj *types.SegJob) error {
	o.Lock()
	defer o.Unlock()

	if o.inStop {
		return xerrors.Errorf("%d is stop", o.pro)
	}

	bjob, ok := o.jobs[sj.BucketID]
	if ok {
		bjob.jobs = append(bjob.jobs, sj)
		o.jobCnt++
	} else {
		bjob = &bucketJob{
			jobs: make([]*types.SegJob, 0, 64),
		}

		bjob.jobs = append(bjob.jobs, sj)
		o.jobCnt++

		o.jobs[sj.BucketID] = bjob
		o.buckets = append(o.buckets, sj.BucketID)
	}

	return nil
}

// jobs on each provider
func (o *proInst) jobCount() int {
	o.RLock()
	defer o.RUnlock()
	return o.jobCnt
}

// send chunk to provider
func (m *OrderMgr) sendChunk(o *proInst) {
	i := 0
	for {
		select {
		case <-m.ctx.Done():
			logger.Info("exit send chunk")
			return
		default:
			if o.inStop {
				time.Sleep(10 * time.Minute)
				logger.Info("cancle send chunk duo to stop at: ", o.pro)
				continue
			}

			if !o.ready {
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

			if o.jobCount() == 0 {
				time.Sleep(time.Second)
				continue
			}

			got := m.sendCtr.TryAcquire(1)
			if !got {
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

			if o.seq.Segments.Has(sj.BucketID, sj.Start, sj.ChunkID) {
				bjob = o.jobs[bid]
				bjob.jobs = bjob.jobs[1:]
				o.jobCnt--
				o.Unlock()
				m.sendCtr.Release(1)
				continue
			}

			o.inflight = true
			o.Unlock()

			sid, err := segment.NewSegmentID(o.fsID, sj.BucketID, sj.Start, sj.ChunkID)
			if err != nil {
				m.sendCtr.Release(1)
				o.Lock()
				o.inflight = false
				o.Unlock()

				time.Sleep(10 * time.Second)
				continue
			}

			// has been sent?
			_, err = o.GetSegmentLocation(o.ctx, sid)
			if err == nil {
				m.sendCtr.Release(1)

				o.Lock()
				bjob = o.jobs[bid]
				bjob.jobs = bjob.jobs[1:]
				o.jobCnt--
				o.inflight = false
				o.Unlock()
				m.segSentChan <- sj

				continue
			}

			err = o.SendSegmentByID(o.ctx, sid, o.pro)
			m.sendCtr.Release(1)

			if err != nil {
				logger.Debug("send segment fail: ", o.pro, sid.GetBucketID(), sid.GetStripeID(), sid.GetChunkID(), err)

				// local has no such block, so skip it
				if strings.Contains(err.Error(), "missing chunk") {
					o.Lock()
					bjob = o.jobs[bid]
					bjob.jobs = bjob.jobs[1:]
					o.jobCnt--
					o.inflight = false
					o.Unlock()

					time.Sleep(1 * time.Second)

					continue
				}

				if !strings.Contains(err.Error(), "already has seg") {
					o.Lock()
					o.inflight = false
					o.Unlock()
					o.failSent++
					time.Sleep(60 * time.Second)
					if strings.Contains(err.Error(), "resource limit exceeded") {
						time.Sleep(2 * time.Minute)
					}
					continue
				}

				// skip chunk if not in recovery mode
				/*
					if strings.Contains(err.Error(), "in local") && os.Getenv("MEFS_RECOVERY_MODE") == "" {
						o.Lock()
						bjob = o.jobs[bid]
						bjob.jobs = bjob.jobs[1:]
						o.inflight = false
						o.Unlock()
						continue
					}
				*/
			} else {
				o.failSent = 0
			}

			logger.Debug("send segment: ", o.pro, sid.GetBucketID(), sid.GetStripeID(), sid.GetChunkID())

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
			o.jobCnt--
			o.inflight = false
			o.Unlock()

			saveOrderSeq(o, m.ds)
			saveSeqJob(o, m.ds)

			m.segSentChan <- sj
		}
	}
}
