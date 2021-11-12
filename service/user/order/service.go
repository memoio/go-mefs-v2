package order

import (
	"context"
	"encoding/binary"
	"math/big"
	"sync"
	"time"

	"github.com/emirpasic/gods/sets/hashset"

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

	orders map[uint64]*OrderFull // key: proID

	proSet  *hashset.Set                  // all pros
	proMap  map[uint64]*lastProsPerBucket // key: bucketID
	proLock sync.RWMutex

	segs    []*segJob
	segLock sync.RWMutex

	newOrderChan chan uint64
	quoChan      chan *types.Quotation
	confirmChan  chan *types.OrderBase
	seqChan      chan *types.OrderSeq

	segAddChan  chan *types.SegJob
	segRedoChan chan *types.SegJob
	segDoneChan chan *types.SegJob

	ready bool
}

func NewOrderMgr(ctx context.Context, roleID uint64, ds store.KVStore, ir api.IRole, in api.INetService, id api.IDataService) *OrderMgr {
	proset := hashset.New()

	om := &OrderMgr{
		IRole:        ir,
		IDataService: id,
		INetService:  in,

		ctx: ctx,

		proSet: proset,

		segPrice: big.NewInt(100),

		newOrderChan: make(chan uint64, 16),
		quoChan:      make(chan *types.Quotation, 16),
		confirmChan:  make(chan *types.OrderBase, 16),
		seqChan:      make(chan *types.OrderSeq, 16),

		segAddChan:  make(chan *types.SegJob, 16),
		segRedoChan: make(chan *types.SegJob, 16),
		segDoneChan: make(chan *types.SegJob, 16),
	}

	om.load()
	om.addPros()

	om.ready = true

	go om.runSched()

	go om.dispatch()

	logger.Info("creaet order manager")

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
		m.proSet.Add(pid)
		m.orders[pid] = m.newOrder(pid)
	}

	return nil
}

func (m *OrderMgr) save(ctx context.Context) error {
	proVal := m.proSet.Values()
	buf := make([]byte, 8*len(proVal))
	for i := 0; i < len(proVal); i++ {
		binary.BigEndian.PutUint64(buf[8*i:8*(i+1)], proVal[i].(uint64))
	}

	key := store.NewKey(pb.MetaType_OrderProsKey, m.localID)
	return m.ds.Put(key, buf)
}

func (m *OrderMgr) addPros() {
	pros := m.IRole.RoleGetRelated(pb.RoleInfo_Provider)
	for _, pro := range pros {
		has := m.proSet.Contains(pro)
		if !has {
			m.orders[pro] = m.newOrder(pro)
			m.proSet.Add(pro)
		}
	}
}

func (m *OrderMgr) runSched() {
	st := time.NewTicker(time.Minute)
	defer st.Stop()

	lt := time.NewTicker(5 * time.Minute)
	defer lt.Stop()
	for {
		select {
		case <-st.C:
		case <-lt.C:
			m.addPros() // add providers

			go m.save(m.ctx)

			m.proLock.RLock()
			for _, lp := range m.proMap {
				m.saveLastProsPerBucket(lp)
			}
			m.proLock.RUnlock()
		case <-m.seqChan:
			// todo
		case <-m.ctx.Done():
			return
		}
	}
}
