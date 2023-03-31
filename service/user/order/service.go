package order

import (
	"context"
	"encoding/binary"
	"math"
	"math/big"
	"os"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/service/netapp"
	"github.com/memoio/go-mefs-v2/submodule/control"
)

type OrderMgr struct {
	api.IRole
	api.IDataService
	api.IRestrict
	api.IChainPush
	api.INetService

	is api.ISettle

	// TODO: remove
	ns *netapp.NetServiceImpl
	ds store.KVStore // save order info

	ctx     context.Context
	sendCtr *semaphore.Weighted

	localID uint64
	fsID    []byte

	orderDur  int64
	orderLast int64
	seqLast   int64
	segPrice  *big.Int
	needPay   *big.Int

	sizelk sync.RWMutex // lock opi update
	opi    *types.OrderPayInfo

	lk         sync.RWMutex // lock orders and proMap update
	inCreation map[uint64]struct{}
	pros       []uint64
	orders     map[uint64]*OrderFull // key: proID
	buckets    []uint64
	proMap     map[uint64]*lastProsPerBucket // key: bucketID

	segs map[jobKey]*segJob

	updateChan chan uint64             // network update chan
	proChan    chan *OrderFull         // provider add chan
	bucketChan chan *lastProsPerBucket // bucket create chan

	quoChan       chan *types.Quotation   // to create new order
	orderChan     chan *types.SignedOrder // confirm new order
	seqNewChan    chan *orderSeqPro       // confirm new seq
	seqFinishChan chan *orderSeqPro       // confirm current seq finish

	// add data
	segAddChan     chan *types.SegJob // add new segjob
	segRedoChan    chan *types.SegJob // todo: check
	segSentChan    chan *types.SegJob // segjob is sent
	segConfirmChan chan *types.SegJob // segjob is confirmed

	// message send out
	msgChan chan *tx.Message

	ready   bool
	inCheck bool
}

func NewOrderMgr(ctx context.Context, roleID uint64, fsID []byte, price, orderDuration, orderLast uint64, ds store.KVStore, pp api.IChainPush, ir api.IRole, id api.IDataService, ns *netapp.NetServiceImpl, is api.ISettle) *OrderMgr {
	if orderLast < 600 {
		logger.Debug("order last is set to 12 hours")
		orderLast = 12 * 3600
	}

	seqLast := orderLast / 10
	if seqLast < 180 {
		seqLast = 180
	}

	if orderDuration < build.OrderMin {
		orderDuration = build.OrderMin
	}

	if orderDuration > build.OrderMax {
		orderDuration = build.OrderMax
	}

	om := &OrderMgr{
		ctx: ctx,

		IRole:        ir,
		IDataService: id,
		IChainPush:   pp,
		INetService:  ns,
		IRestrict:    control.New(ds),

		ds: ds,
		ns: ns,
		is: is,

		sendCtr: semaphore.NewWeighted(defaultWeighted),

		localID: roleID,
		fsID:    fsID,

		orderDur:  int64(orderDuration),
		orderLast: int64(orderLast),
		seqLast:   int64(seqLast),

		segPrice: new(big.Int).SetUint64(price),
		needPay:  big.NewInt(0),

		opi: &types.OrderPayInfo{
			NeedPay: big.NewInt(0),
			Paid:    big.NewInt(0),
		},

		inCreation: make(map[uint64]struct{}),
		pros:       make([]uint64, 0, 128),
		orders:     make(map[uint64]*OrderFull),
		buckets:    make([]uint64, 0, 16),
		proMap:     make(map[uint64]*lastProsPerBucket),

		segs: make(map[jobKey]*segJob),

		proChan:    make(chan *OrderFull, 8),
		bucketChan: make(chan *lastProsPerBucket),

		updateChan: make(chan uint64, 128),

		quoChan:       make(chan *types.Quotation, 128),
		orderChan:     make(chan *types.SignedOrder, 128),
		seqNewChan:    make(chan *orderSeqPro, 128),
		seqFinishChan: make(chan *orderSeqPro, 128),

		segAddChan:     make(chan *types.SegJob, 128),
		segRedoChan:    make(chan *types.SegJob, 128),
		segSentChan:    make(chan *types.SegJob, 128),
		segConfirmChan: make(chan *types.SegJob, 128),

		msgChan: make(chan *tx.Message, 1024),
	}

	logger.Info("create order manager")

	return om
}

func (m *OrderMgr) Start() {
	m.load()

	m.ready = true

	go m.runProSched()
	go m.runSegSched()
	go m.runSched()

	go m.runPush()
	go m.runCheck()
}

func (m *OrderMgr) Stop() {
	logger.Info("stop order manager")
	m.lk.Lock()
	defer m.lk.Unlock()
	m.ready = false
}

func (m *OrderMgr) load() error {
	key := store.NewKey(pb.MetaType_OrderPayInfoKey, m.localID)
	val, err := m.ds.Get(key)
	if err == nil {
		m.opi.Deserialize(val)
	}

	size := uint64(0)
	pros, err := m.StateGetProsAt(context.TODO(), m.localID)
	if err == nil {
		for _, pid := range pros {
			si, err := m.is.SettleGetStoreInfo(m.ctx, m.localID, pid)
			if err != nil {
				continue
			}
			size += si.Size

			popi := new(types.OrderPayInfo)
			key := store.NewKey(pb.MetaType_OrderPayInfoKey, m.localID, pid)
			val, err := m.ds.Get(key)
			if err == nil {
				popi.Deserialize(val)
			}

			if popi.OnChainSize < si.Size || popi.ConfirmSize == 0 {
				popi.OnChainSize = si.Size
				if popi.ConfirmSize == 0 {
					popi.ConfirmSize = si.Size
				}
				if popi.OnChainSize == popi.Size {
					popi.Paid.Set(popi.NeedPay)
				}

				key := store.NewKey(pb.MetaType_OrderPayInfoKey, m.localID, pid)
				val, _ := popi.Serialize()
				m.ds.Put(key, val)
			}
		}
		m.opi.OnChainSize = size
		if m.opi.OnChainSize == m.opi.Size {
			m.opi.Paid.Set(m.opi.NeedPay)
		}
		key = store.NewKey(pb.MetaType_OrderPayInfoKey, m.localID)
		val, _ = m.opi.Serialize()
		m.ds.Put(key, val)
	}

	if os.Getenv("MEFS_RECOVERY_MODE") != "" {
		m.opi.Size = 0
		m.opi.NeedPay = new(big.Int)
		m.opi.ConfirmSize = 0
	}

	key = store.NewKey(pb.MetaType_OrderProsKey, m.localID)
	val, err = m.ds.Get(key)
	if err == nil {
		for i := 0; i < len(val)/8; i++ {
			pid := binary.BigEndian.Uint64(val[8*i : 8*(i+1)])
			pros = append(pros, pid)
		}
	}

	pros = removeDup(pros)

	for _, pid := range pros {
		go m.newProOrder(pid)
	}
	logger.Info("load pros: ", len(pros))

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
	logger.Debug("expand pros: ", len(pros), pros)
	for _, pro := range pros {
		has := false
		for _, pid := range m.pros {
			if pid == pro {
				has = true
			}
		}

		if !has {
			go m.newProOrder(pro)
		}
	}
}

// add and update pro
func (m *OrderMgr) runProSched() {
	lt := time.NewTicker(5 * time.Minute)
	defer lt.Stop()

	m.addPros() // add providers

	for {
		select {
		case of := <-m.proChan:
			logger.Debug("add order to pro: ", of.pro)
			_, ok := m.orders[of.pro]
			if !ok {
				m.lk.Lock()
				m.orders[of.pro] = of
				m.pros = append(m.pros, of.pro)
				m.lk.Unlock()
				go m.update(of.pro)
				go m.sendChunk(of)
				go m.runOrderSched(of)
			}
		case lp := <-m.bucketChan:
			logger.Debug("add bucket: ", lp.bucketID)
			_, ok := m.proMap[lp.bucketID]
			if !ok {
				m.buckets = append(m.buckets, lp.bucketID)
				m.proMap[lp.bucketID] = lp
			}
		case pid := <-m.updateChan:
			logger.Debug("update pro time: ", pid)
			m.lk.RLock()
			of, ok := m.orders[pid]
			m.lk.RUnlock()
			if ok {
				of.availTime = time.Now().Unix()
			} else {
				go m.newProOrder(pid)
			}
		case <-lt.C:
			logger.Debug("update pros in bucket")
			m.addPros() // add providers

			expand := false
			if len(m.pros) == 0 {
				expand = true
			}

			for _, pid := range m.pros {
				go m.RoleGet(m.ctx, pid, true)
			}

			for _, bid := range m.buckets {
				lp, ok := m.proMap[bid]
				if ok {
					m.updateProsForBucket(lp)

					if expand {
						continue
					}

					for _, pid := range lp.pros {
						if pid == math.MaxUint64 {
							expand = true
							break
						}
					}
				}
			}

			if expand {
				go m.IRole.RoleExpand(m.ctx)
			}

			m.save()
		case <-m.ctx.Done():
			return
		}
	}
}

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

// handle msg over net, then dispatch to provider order
func (m *OrderMgr) runSched() {
	for {
		select {
		// handle order state
		case quo := <-m.quoChan:
			m.lk.RLock()
			of, ok := m.orders[quo.ProID]
			m.lk.RUnlock()
			if ok {
				of.quoChan <- quo
			}
		case ob := <-m.orderChan:
			m.lk.RLock()
			of, ok := m.orders[ob.ProID]
			m.lk.RUnlock()
			if ok {
				of.orderChan <- ob
			}
		case s := <-m.seqNewChan:
			m.lk.RLock()
			of, ok := m.orders[s.proID]
			m.lk.RUnlock()
			if ok {
				of.seqNewChan <- s
			}
		case s := <-m.seqFinishChan:
			m.lk.RLock()
			of, ok := m.orders[s.proID]
			m.lk.RUnlock()
			if ok {
				of.seqFinishChan <- s
			}
		case <-m.ctx.Done():
			return
		}
	}
}

// check each provider's state
func (m *OrderMgr) runOrderSched(of *OrderFull) {
	st := time.NewTicker(59 * time.Second)
	defer st.Stop()

	lt := time.NewTicker(10 * time.Minute)
	defer lt.Stop()

	for {
		select {
		// handle order state
		case quo := <-of.quoChan:
			if quo.SegPrice.Cmp(m.segPrice) > 0 {
				of.failCnt++
			} else {
				of.failCnt = 0
				of.availTime = time.Now().Unix()
				err := m.createOrder(of, quo)
				if err != nil {
					logger.Debug("fail create new order: ", err)
				}
			}
		case ob := <-of.orderChan:
			of.availTime = time.Now().Unix()
			err := m.runOrder(of, ob)
			if err != nil {
				logger.Debug("fail run new order: ", ob.ProID, err)
			}
		case s := <-of.seqNewChan:
			of.availTime = time.Now().Unix()
			err := m.sendSeq(of, s.os)
			if err != nil {
				logger.Debug("fail send new seq: ", err)
			}
		case s := <-of.seqFinishChan:
			of.availTime = time.Now().Unix()
			err := m.finishSeq(of, s.os)
			if err != nil {
				logger.Debug("fail finish seq: ", err)
			}
		case <-st.C:
			m.check(of)
		case <-lt.C:
			ri, err := m.RoleGet(m.ctx, of.pro, true)
			if err == nil {
				of.location = string(ri.GetDesc())
			}

		case <-m.ctx.Done():
			return
		}
	}
}
