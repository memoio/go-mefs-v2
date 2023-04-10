package order

import (
	"context"
	"encoding/binary"
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

	// todo: remove
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

	lk         sync.RWMutex // lock orders and bucMap update
	inCreation map[uint64]time.Time
	pros       []uint64
	pInstMap   map[uint64]*proInst // key: proID
	buckets    []uint64
	bucMap     map[uint64]*lastProsPerBucket // key: bucketID

	segs map[jobKey]*segJob

	proChan    chan *proInst           // provider add chan
	bucketChan chan *lastProsPerBucket // bucket create chan

	updateChan    chan uint64 // network update chan
	failChan      chan uint64
	quoChan       chan *types.Quotation   // to create new order
	orderChan     chan *types.SignedOrder // confirm new order
	seqNewChan    chan *orderSeqPro       // confirm new seq
	seqFinishChan chan *orderSeqPro       // confirm current seq finish

	// add data
	segAddChan     chan *types.SegJob // add new segjob
	segRedoChan    chan *types.SegJob // redo segjob when load or stop
	segSentChan    chan *types.SegJob // segjob is sent
	segConfirmChan chan *types.SegJob // segjob is confirmed

	// message send out
	msgChan chan *tx.Message

	expand  bool
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

		inCreation: make(map[uint64]time.Time),
		pros:       make([]uint64, 0, 128),
		pInstMap:   make(map[uint64]*proInst),
		buckets:    make([]uint64, 0, 16),
		bucMap:     make(map[uint64]*lastProsPerBucket),

		segs: make(map[jobKey]*segJob),

		bucketChan: make(chan *lastProsPerBucket),

		proChan:       make(chan *proInst, 128),
		updateChan:    make(chan uint64, 128),
		failChan:      make(chan uint64, 128),
		quoChan:       make(chan *types.Quotation, 128),
		orderChan:     make(chan *types.SignedOrder, 128),
		seqNewChan:    make(chan *orderSeqPro, 128),
		seqFinishChan: make(chan *orderSeqPro, 128),

		segAddChan:     make(chan *types.SegJob, 128),
		segRedoChan:    make(chan *types.SegJob, 128),
		segSentChan:    make(chan *types.SegJob, 128),
		segConfirmChan: make(chan *types.SegJob, 128),

		msgChan: make(chan *tx.Message, 1024),

		expand: true,
	}

	logger.Info("create order manager")

	return om
}

func (m *OrderMgr) Start() {
	m.load()

	m.ready = true

	go m.runProSched()
	go m.runSegSched()
	go m.runNetSched()
	go m.runBucketSched()

	go m.runPush()
	go m.runBalCheck()
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

	// load size from settle chain
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
				val, err := popi.Serialize()
				if err == nil {
					m.ds.Put(key, val)
				}
			}
		}
		m.opi.OnChainSize = size
		if m.opi.OnChainSize == m.opi.Size {
			m.opi.Paid.Set(m.opi.NeedPay)
		}
		key = store.NewKey(pb.MetaType_OrderPayInfoKey, m.localID)
		val, err = m.opi.Serialize()
		if err == nil {
			m.ds.Put(key, val)
		}
	}

	if os.Getenv("MEFS_RECOVERY_MODE") == "rebuild" {
		// reset for later reget
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
		go m.createProOrder(pid)
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
