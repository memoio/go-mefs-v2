package order

import (
	"context"
	"encoding/binary"
	"math/big"
	"sync"
	"time"

	"github.com/jbenet/goprocess"
	goprocessctx "github.com/jbenet/goprocess/context"
	"golang.org/x/sync/semaphore"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/service/netapp"
	"github.com/memoio/go-mefs-v2/submodule/connect/settle"
	"github.com/memoio/go-mefs-v2/submodule/txPool"
)

type OrderMgr struct {
	api.IRole
	api.IDataService
	*txPool.PushPool

	lk      sync.RWMutex
	ctx     context.Context
	proc    goprocess.Process
	sendCtr *semaphore.Weighted

	ds store.KVStore // save order info

	is      *settle.ContractMgr
	ns      *netapp.NetServiceImpl
	localID uint64
	fsID    []byte

	segPrice *big.Int
	needPay  *big.Int

	pros   []uint64
	orders map[uint64]*OrderFull         // key: proID
	proMap map[uint64]*lastProsPerBucket // key: bucketID

	segs    map[jobKey]*segJob
	segLock sync.RWMutex // for get state of job

	proChan    chan *OrderFull
	bucketChan chan *lastProsPerBucket

	updateChan chan uint64

	quoChan       chan *types.Quotation   // to init
	orderChan     chan *types.SignedOrder // confirm new order
	seqNewChan    chan *orderSeqPro       // confirm new seq
	seqFinishChan chan *orderSeqPro       // confirm current seq

	// add data
	segAddChan  chan *types.SegJob
	segRedoChan chan *types.SegJob
	segDoneChan chan *types.SegJob

	// message send out
	msgChan chan *tx.Message

	ready   bool
	inCheck bool
}

func NewOrderMgr(ctx context.Context, roleID uint64, fsID []byte, ds store.KVStore, pp *txPool.PushPool, ir api.IRole, id api.IDataService, ns *netapp.NetServiceImpl, is *settle.ContractMgr) *OrderMgr {
	om := &OrderMgr{
		IRole:        ir,
		IDataService: id,
		PushPool:     pp,

		ds: ds,
		ns: ns,
		is: is,

		sendCtr: semaphore.NewWeighted(defaultWeighted),

		localID: roleID,
		fsID:    fsID,

		segPrice: new(big.Int).Set(build.DefaultSegPrice),
		needPay:  big.NewInt(0),

		orders: make(map[uint64]*OrderFull),
		proMap: make(map[uint64]*lastProsPerBucket),

		segs: make(map[jobKey]*segJob),

		proChan:    make(chan *OrderFull, 8),
		bucketChan: make(chan *lastProsPerBucket),

		updateChan: make(chan uint64, 16),

		quoChan:       make(chan *types.Quotation, 16),
		orderChan:     make(chan *types.SignedOrder, 16),
		seqNewChan:    make(chan *orderSeqPro, 16),
		seqFinishChan: make(chan *orderSeqPro, 16),

		segAddChan:  make(chan *types.SegJob, 128),
		segRedoChan: make(chan *types.SegJob, 128),
		segDoneChan: make(chan *types.SegJob, 128),

		msgChan: make(chan *tx.Message, 128),
	}

	om.proc = goprocessctx.WithContext(ctx)
	om.ctx = goprocessctx.WithProcessClosing(ctx, om.proc)

	logger.Info("create order manager")

	return om
}

func (m *OrderMgr) Start() {
	m.load()

	m.ready = true

	m.proc.Go(m.runSched)

	m.proc.Go(m.runPush)
	m.proc.Go(m.runCheck)

	go m.dispatch()
}

func (m *OrderMgr) Stop() {
	m.lk.Lock()
	defer m.lk.Unlock()
	m.ready = false
	m.proc.Close()
}

func (m *OrderMgr) load() error {
	key := store.NewKey(pb.MetaType_OrderProsKey, m.localID)
	val, err := m.ds.Get(key)
	if err != nil {
		return err
	}

	res := make([]uint64, len(val)/8)
	for i := 0; i < len(val)/8; i++ {
		pid := binary.BigEndian.Uint64(val[8*i : 8*(i+1)])
		res[i] = pid
	}

	res = removeDup(res)

	for _, pid := range res {
		go m.newProOrder(pid)
	}

	return nil
}

func (m *OrderMgr) save() error {
	buf := make([]byte, 8*len(m.pros))
	for i, pid := range m.pros {
		binary.BigEndian.PutUint64(buf[8*i:8*(i+1)], pid)
	}

	key := store.NewKey(pb.MetaType_OrderProsKey, m.localID)
	return m.ds.Put(key, buf)
}

func (m *OrderMgr) addPros() {
	pros, _ := m.IRole.RoleGetRelated(m.ctx, pb.RoleInfo_Provider)
	for _, pro := range pros {
		has := false
		for _, pid := range m.pros {
			if pid == pro {
				has = true
			}
		}

		if !has {
			go m.update(pro)
		}
	}

}

func (m *OrderMgr) runSched(proc goprocess.Process) {
	st := time.NewTicker(10 * time.Second)
	defer st.Stop()

	lt := time.NewTicker(5 * time.Minute)
	defer lt.Stop()

	m.addPros() // add providers

	for {
		// handle data
		select {
		case sj := <-m.segAddChan:
			m.addSegJob(sj)
		case sj := <-m.segRedoChan:
			m.redoSegJob(sj)
		case sj := <-m.segDoneChan:
			m.finishSegJob(sj)

		// handle order state
		case quo := <-m.quoChan:
			of, ok := m.orders[quo.ProID]
			if ok {
				if quo.SegPrice.Cmp(m.segPrice) > 0 {
					of.quoretry += 1
					if of.quoretry > 10 {
						m.stopOrder(of)
					}
				} else {
					of.quoretry = 0
					of.availTime = time.Now().Unix()
					err := m.createOrder(of, quo)
					if err != nil {
						logger.Debug("fail create new order:", err)
					}
				}
			}
		case ob := <-m.orderChan:
			of, ok := m.orders[ob.ProID]
			if ok {
				of.availTime = time.Now().Unix()
				err := m.runOrder(of, ob)
				if err != nil {
					logger.Debug("fail run new order:", ob.ProID, err)
				}
			}
		case s := <-m.seqNewChan:
			of, ok := m.orders[s.proID]
			if ok {
				of.availTime = time.Now().Unix()
				err := m.sendSeq(of, s.os)
				if err != nil {
					logger.Debug("fail send new seq:", err)
				}
			}
		case s := <-m.seqFinishChan:
			of, ok := m.orders[s.proID]
			if ok {
				of.availTime = time.Now().Unix()
				err := m.finishSeq(of, s.os)
				if err != nil {
					logger.Debug("fail finish seq:", err)
				}
			}
		case of := <-m.proChan:
			logger.Debug("add order to pro:", of.pro)
			_, ok := m.orders[of.pro]
			if !ok {
				m.orders[of.pro] = of
				m.pros = append(m.pros, of.pro)
				go m.update(of.pro)
			}

		case lp := <-m.bucketChan:
			_, ok := m.proMap[lp.bucketID]
			if !ok {
				m.proMap[lp.bucketID] = lp
			}

		case pid := <-m.updateChan:
			of, ok := m.orders[pid]
			if ok {
				of.availTime = time.Now().Unix()
			} else {
				go m.newProOrder(pid)
			}
		case <-st.C:
			// dispatch to each pro
			m.dispatch()

			for _, pid := range m.pros {
				of := m.orders[pid]
				m.check(of)
			}
		case <-lt.C:
			m.addPros() // add providers

			m.save()

			for _, lp := range m.proMap {
				m.saveLastProsPerBucket(lp)
			}
		case <-proc.Closing():
			return
		}
	}
}
