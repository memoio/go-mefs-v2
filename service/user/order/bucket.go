package order

import (
	"encoding/binary"
	"math"
	"os"
	"sync"
	"time"

	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/lib/utils"
)

// update providers in bucket

type lastProsPerBucket struct {
	lk          sync.RWMutex
	bucketID    uint64
	dc, pc      int
	pros        []uint64 // update and save to local
	deleted     []uint64 // add del pro here
	delPerChunk [][]uint64
}

// TODO: change pro when quotation price is too high
// TODO: fix order duplicated due to pro change

func (m *OrderMgr) RegisterBucket(bucketID, nextOpID uint64, bopt *pb.BucketOption) {
	logger.Info("register order for bucket: ", bucketID, nextOpID)

	storedPros, delPros, delPer := m.loadLastProsPerBucket(bucketID, int(bopt.DataCount+bopt.ParityCount))

	logger.Info("load order bucket local: ", bucketID, storedPros, delPros, delPer)

	lp := &lastProsPerBucket{
		bucketID:    bucketID,
		dc:          int(bopt.DataCount),
		pc:          int(bopt.ParityCount),
		pros:        storedPros,
		deleted:     delPros,
		delPerChunk: delPer,
	}

	// load all pros used in bucket
	for _, pid := range lp.pros {
		go m.newProOrder(pid)
	}

	time.Sleep(30 * time.Second)

	m.updateProsForBucket(lp)

	logger.Info("load order bucket: ", lp.bucketID, lp.pros, lp.deleted)

	m.bucketChan <- lp

	// wait order suc
	m.loadUnfinishedSegJobs(bucketID, nextOpID)
}

func (m *OrderMgr) loadLastProsPerBucket(bucketID uint64, cnt int) ([]uint64, []uint64, [][]uint64) {
	res := make([]uint64, cnt)
	delRes := make([]uint64, 0, 1)
	delPer := make([][]uint64, cnt)

	for i := 0; i < int(cnt); i++ {
		res[i] = math.MaxUint64
		delPer[i] = make([]uint64, 0, 1)
	}

	key := store.NewKey(pb.MetaType_OrderProsKey, m.localID, bucketID)
	val, err := m.ds.Get(key)
	if err != nil {
		return res, delRes, delPer
	}

	for i := 0; i < len(val)/8 && i < cnt; i++ {
		pid := binary.BigEndian.Uint64(val[8*i : 8*(i+1)])
		if pid != math.MaxUint64 {
			go m.newProOrder(pid)
		}

		res[i] = pid
	}

	key = store.NewKey(pb.MetaType_OrderProsDeleteKey, m.localID, bucketID)
	val, err = m.ds.Get(key)
	if err != nil {
		return res, delRes, delPer
	}

	delRes = make([]uint64, len(val)/8)
	for i := 0; i < len(val)/8; i++ {
		pid := binary.BigEndian.Uint64(val[8*i : 8*(i+1)])
		if pid != math.MaxUint64 {
			delRes[i] = pid
		}
	}

	for i := 0; i < cnt; i++ {
		key = store.NewKey(pb.MetaType_OrderProsDeleteKey, m.localID, bucketID, i)
		val, err = m.ds.Get(key)
		if err != nil {
			continue
		}

		for j := 0; j < len(val)/8; j++ {
			pid := binary.BigEndian.Uint64(val[8*j : 8*(j+1)])
			if pid != math.MaxUint64 {
				delPer[i] = append(delPer[i], pid)
			}
		}
	}

	return res, delRes, delPer
}

func (m *OrderMgr) saveLastProsPerBucket(lp *lastProsPerBucket) {
	buf := make([]byte, 8*len(lp.pros))
	for i, pid := range lp.pros {
		binary.BigEndian.PutUint64(buf[8*i:8*(i+1)], pid)
	}

	key := store.NewKey(pb.MetaType_OrderProsKey, m.localID, lp.bucketID)
	m.ds.Put(key, buf)

	if len(lp.deleted) != 0 {
		buf = make([]byte, 8*len(lp.deleted))
		for i, pid := range lp.deleted {
			binary.BigEndian.PutUint64(buf[8*i:8*(i+1)], pid)
		}

		key = store.NewKey(pb.MetaType_OrderProsDeleteKey, m.localID, lp.bucketID)
		m.ds.Put(key, buf)
	}

	for i := 0; i < len(lp.pros); i++ {
		if len(lp.delPerChunk[i]) == 0 {
			continue
		}
		buf = make([]byte, 8*len(lp.delPerChunk[i]))
		for j, pid := range lp.delPerChunk[i] {
			binary.BigEndian.PutUint64(buf[8*j:8*(j+1)], pid)
		}

		key = store.NewKey(pb.MetaType_OrderProsDeleteKey, m.localID, lp.bucketID, i)
		m.ds.Put(key, buf)
	}
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
	if !m.ready {
		return
	}

	cnt := 0
	for _, pid := range lp.pros {
		if pid == math.MaxUint64 {
			continue
		}

		if !m.RestrictHas(m.ctx, pid) {
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

	logger.Debugf("order bucket %d expected %d, got %d", lp.bucketID, lp.dc+lp.pc, cnt)

	if cnt >= lp.dc+lp.pc {
		return
	}

	// expand providers

	cloudPros := make([]uint64, 0, len(m.pros))
	personPros := make([]uint64, 0, len(m.pros))
	for _, pid := range m.pros {
		if !m.RestrictHas(m.ctx, pid) {
			continue
		}

		hasDelete := false
		for _, dpid := range lp.deleted {
			if dpid == math.MaxUint64 {
				continue
			}
			if pid == dpid {
				hasDelete = true
				break
			}
		}

		if hasDelete {
			continue
		}

		// not deleted
		has := false
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
				if or.location == "cloud" {
					cloudPros = append(cloudPros, pid)
				} else {
					personPros = append(personPros, pid)
				}
			}
		}
	}

	utils.DisorderUint(cloudPros)
	utils.DisorderUint(personPros)

	lp.lk.Lock()
	defer lp.lk.Unlock()

	change := false
	j := 0

	switch os.Getenv("MEFS_PRO_SELECT_POLICY") {
	case "strict":
		for i := 0; i < lp.dc; i++ {
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

			for j < len(cloudPros) {
				npid := cloudPros[j]
				j++
				m.lk.RLock()
				or, ok := m.orders[npid]
				m.lk.RUnlock()
				if ok {
					// choose good ones
					if or.isGood() {
						change = true
						lp.pros[i] = npid
						if pid != math.MaxUint64 {
							lp.deleted = append(lp.deleted, pid)
							lp.delPerChunk[i] = append(lp.delPerChunk[i], pid)
						}
						break
					}
				}
			}
		}

		j = 0
		for i := lp.dc; i < lp.dc+lp.pc; i++ {
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

			for j < len(personPros) {
				npid := personPros[j]
				j++
				m.lk.RLock()
				or, ok := m.orders[npid]
				m.lk.RUnlock()
				if ok {
					if or.isGood() {
						change = true
						lp.pros[i] = npid
						if pid != math.MaxUint64 {
							lp.deleted = append(lp.deleted, pid)
							lp.delPerChunk[i] = append(lp.delPerChunk[i], pid)
						}
						break
					}
				}
			}
		}
	default:

		if len(cloudPros) > lp.dc {
			personPros = append(personPros, cloudPros[lp.dc:]...)
			cloudPros = cloudPros[:lp.dc]
		}
		cloudPros = append(cloudPros, personPros...)

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

			for j < len(cloudPros) {
				npid := cloudPros[j]
				j++
				m.lk.RLock()
				or, ok := m.orders[npid]
				m.lk.RUnlock()
				if ok {
					if or.isGood() {
						change = true
						lp.pros[i] = npid
						if pid != math.MaxUint64 {
							lp.deleted = append(lp.deleted, pid)
							lp.delPerChunk[i] = append(lp.delPerChunk[i], pid)
						}

						break
					}
				}
			}
		}
	}

	if change {
		m.saveLastProsPerBucket(lp)
		logger.Info("order bucket: ", lp.bucketID, lp.pros)
	}

	logger.Debug("order bucket: ", lp.bucketID, lp.pros, lp.deleted, lp.delPerChunk)
}

func (o *OrderFull) isGood() bool {
	if !o.inStop && o.ready && o.failCnt < 30 && o.failSent < 30 {
		return true
	}

	return false
}
