package order

import (
	"context"
	"encoding/binary"
	"math/big"
	"sync"
	"time"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

type OrderMgr struct {
	api.IRole
	api.INetService
	api.IDataService

	ctx context.Context
	ds  store.KVStore // save order info

	localID uint64
	fsID    []byte

	segPrice *big.Int

	orders map[uint64]*OrderFull         // key: proID
	proMap map[uint64]*lastProsPerBucket // key: bucketID

	segs    map[jobKey]*segJob
	segLock sync.RWMutex // for get state of job

	proChan    chan *OrderFull
	bucketChan chan *lastProsPerBucket

	quoChan       chan *types.Quotation // to init
	orderChan     chan *types.OrderBase // confirm new order
	seqNewChan    chan *orderSeqPro     // confirm new seq
	seqFinishChan chan *orderSeqPro     // confirm current seq

	// add data
	segAddChan  chan *types.SegJob
	segRedoChan chan *types.SegJob
	segDoneChan chan *types.SegJob

	ready bool
}

func NewOrderMgr(ctx context.Context, roleID uint64, fsID []byte, ds store.KVStore, ir api.IRole, in api.INetService, id api.IDataService) *OrderMgr {

	om := &OrderMgr{
		IRole:        ir,
		IDataService: id,
		INetService:  in,

		ctx: ctx,
		ds:  ds,

		localID: roleID,
		fsID:    fsID,

		segPrice: big.NewInt(100),

		orders: make(map[uint64]*OrderFull),
		proMap: make(map[uint64]*lastProsPerBucket),

		segs: make(map[jobKey]*segJob),

		proChan:    make(chan *OrderFull, 8),
		bucketChan: make(chan *lastProsPerBucket),

		quoChan:       make(chan *types.Quotation, 16),
		orderChan:     make(chan *types.OrderBase, 16),
		seqNewChan:    make(chan *orderSeqPro, 16),
		seqFinishChan: make(chan *orderSeqPro, 16),

		segAddChan:  make(chan *types.SegJob, 16),
		segRedoChan: make(chan *types.SegJob, 16),
		segDoneChan: make(chan *types.SegJob, 16),
	}

	om.load()

	om.ready = true

	go om.runSched()

	go om.dispatch()

	logger.Info("create order manager")

	return om
}

func (m *OrderMgr) load() error {
	key := store.NewKey(pb.MetaType_OrderProsKey, m.localID)
	val, err := m.ds.Get(key)
	if err != nil {
		return err
	}

	for i := 0; i < len(val)/8; i++ {
		pid := binary.BigEndian.Uint64(val[8*i : 8*(i+1)])
		m.orders[pid] = m.loadProOrder(pid)
	}

	pros := m.IRole.RoleGetRelated(pb.RoleInfo_Provider)
	for _, pid := range pros {
		has := false
		for pro := range m.orders {
			if pid == pro {
				has = true
			}
		}

		if !has {
			m.orders[pid] = m.loadProOrder(pid)
		}
	}

	return nil
}

func (m *OrderMgr) save() error {
	buf := make([]byte, 8*len(m.orders))
	i := 0
	for pid := range m.orders {
		binary.BigEndian.PutUint64(buf[8*i:8*(i+1)], pid)
		i++
	}

	key := store.NewKey(pb.MetaType_OrderProsKey, m.localID)
	return m.ds.Put(key, buf)
}

func (m *OrderMgr) addPros() {
	pros := m.IRole.RoleGetRelated(pb.RoleInfo_Provider)
	for _, pro := range pros {
		has := false
		for pid := range m.orders {
			if pid == pro {
				has = true
			}
		}

		if !has {
			go m.newProOrder(pro)
		}
	}

}

func (m *OrderMgr) runSched() {
	st := time.NewTicker(time.Minute)
	defer st.Stop()

	lt := time.NewTicker(5 * time.Minute)
	defer lt.Stop()

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
				of.availTime = time.Now().Unix()
				m.createOrder(of, quo)
			}
		case ob := <-m.orderChan:
			of, ok := m.orders[ob.ProID]
			if ok {
				of.availTime = time.Now().Unix()
				m.runOrder(of, ob)
			}
		case s := <-m.seqNewChan:
			of, ok := m.orders[s.proID]
			if ok {
				of.availTime = time.Now().Unix()
				m.sendSeq(of, s.os)
			}
		case s := <-m.seqFinishChan:
			of, ok := m.orders[s.proID]
			if ok {
				of.availTime = time.Now().Unix()
				m.finishSeq(of, s.os)
			}
		case of := <-m.proChan:
			m.orders[of.pro] = of
		case lp := <-m.bucketChan:
			_, ok := m.proMap[lp.bucketID]
			if ok {
				m.proMap[lp.bucketID] = lp
			}
		case <-st.C:
			for _, of := range m.orders {
				m.check(of)
			}
		case <-lt.C:
			m.addPros() // add providers

			m.save()

			for _, lp := range m.proMap {
				m.saveLastProsPerBucket(lp)
			}
		case <-m.ctx.Done():
			return
		default:
			// dispatch to each pro
			m.dispatch()
		}
	}
}
