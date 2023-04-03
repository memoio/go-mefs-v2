package order

import (
	"context"
	"math"
	"math/big"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

// per provider
type proInst struct {
	sync.RWMutex

	api.IDataService // segment

	ctx context.Context

	localID  uint64
	fsID     []byte
	location string

	pro        uint64
	availTime  int64 // last connect time
	updateTime int64

	nonce   uint64 // next nonce
	seqNum  uint32 // next seq
	prevEnd int64

	opi *types.OrderPayInfo

	base       *types.SignedOrder // quotation-> base
	orderTime  int64
	orderState OrderState

	seq      *types.SignedOrderSeq
	seqTime  int64
	seqState OrderSeqState

	sjq *types.SegJobsQueue

	inflight bool // data is sending
	buckets  []uint64
	jobs     map[uint64]*bucketJob // buf and persist?
	jobCnt   int

	failCnt  int // TODO: retry > 128; change pro?
	failSent int

	ready  bool // ready for service; network is ok
	inStop bool // stop receiving data; duo to high price

	quoChan       chan *types.Quotation   // to init
	orderChan     chan *types.SignedOrder // confirm new order
	seqNewChan    chan *orderSeqPro       // confirm new seq
	seqFinishChan chan *orderSeqPro       // confirm current seq
}

// add and update pro
func (m *OrderMgr) runProSched() {
	lt := time.NewTicker(5 * time.Minute)
	defer lt.Stop()

	m.addPros() // add providers

	for {
		select {
		case of := <-m.proChan:
			logger.Debug("add proInst: ", of.pro)
			_, ok := m.pInstMap[of.pro]
			if !ok {
				logger.Info("create proinst sat: ", of.pro, of.nonce, of.seqNum, of.orderState, of.seqState)
				m.lk.Lock()
				m.pInstMap[of.pro] = of
				m.pros = append(m.pros, of.pro)
				m.lk.Unlock()
				m.startProInst(of)
			}
		case <-lt.C:
			logger.Debug("add new pros")
			m.addPros() // add providers

			if len(m.pros) == 0 {
				m.expand = true
			}

			for _, pid := range m.pros {
				go m.RoleGet(m.ctx, pid, true)
			}

			if m.expand {
				go m.IRole.RoleExpand(m.ctx)
				m.expand = false
			}

			m.save()
		case <-m.ctx.Done():
			return
		}
	}
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
			go m.createProOrder(pro)
		}
	}
}

func (m *OrderMgr) createProOrder(id uint64) {
	if id == math.MaxUint64 {
		return
	}

	if filterProList(id) {
		return
	}

	m.lk.Lock()
	_, has := m.pInstMap[id]
	if has {
		m.lk.Unlock()
		return
	}

	_, ok := m.inCreation[id]
	if ok {
		m.lk.Unlock()
		return
	}

	m.inCreation[id] = struct{}{}
	m.lk.Unlock()

	logger.Debug("create proInst sat: ", id)
	of := &proInst{
		IDataService: m.IDataService,

		ctx: m.ctx,

		localID: m.localID,
		fsID:    m.fsID,
		pro:     id,

		availTime: time.Now().Unix() - 301,

		prevEnd: time.Now().Unix(),

		opi: new(types.OrderPayInfo),

		orderState: Order_Init,
		seqState:   OrderSeq_Init,
		sjq:        new(types.SegJobsQueue),

		buckets: make([]uint64, 0, 8),
		jobs:    make(map[uint64]*bucketJob),

		quoChan:       make(chan *types.Quotation, 8),
		orderChan:     make(chan *types.SignedOrder, 8),
		seqNewChan:    make(chan *orderSeqPro, 8),
		seqFinishChan: make(chan *orderSeqPro, 8),
	}
	err := m.loadProInst(of)
	logger.Debug("load proInst sat: ", of.pro, of.nonce, of.seqNum, of.orderState, of.seqState, err)

	m.fix(of)

	m.proChan <- of
}

func filterProList(id uint64) bool {
	if os.Getenv("PROLIST") != "" {
		prolist := strings.Split(os.Getenv("PROLIST"), ",")
		for _, pro := range prolist {
			proi, _ := strconv.Atoi(pro)
			if uint64(proi) == id {
				return false
			}
		}
		return true
	}
	return false
}

func (m *OrderMgr) loadProInst(of *proInst) error {
	ri, err := m.RoleGet(m.ctx, of.pro, true)
	if err == nil {
		of.location = string(ri.GetDesc())
	}

	err = m.connect(of.pro)
	if err == nil {
		of.ready = true
	}

	key := store.NewKey(pb.MetaType_OrderPayInfoKey, m.localID, of.pro)
	val, err := m.ds.Get(key)
	if err == nil {
		of.opi.Deserialize(val)
	} // recal iter all orders

	if of.opi.NeedPay == nil {
		of.opi.NeedPay = new(big.Int)
	}

	if of.opi.Paid == nil {
		of.opi.Paid = new(big.Int)
	}

	if of.opi.Balance == nil {
		of.opi.Balance = new(big.Int)
	}

	ns := new(NonceState)
	key = store.NewKey(pb.MetaType_OrderNonceKey, m.localID, of.pro)
	val, err = m.ds.Get(key)
	if err != nil {
		return err
	}
	err = ns.Deserialize(val)
	if err != nil {
		return err
	}

	if ns.State == Order_Init || ns.State == Order_Wait {
		of.nonce = ns.Nonce
	} else {
		of.nonce = ns.Nonce + 1
	}

	of.orderTime = ns.Time
	of.orderState = ns.State

	ob := new(types.SignedOrder)
	key = store.NewKey(pb.MetaType_OrderBaseKey, m.localID, of.pro, ns.Nonce)
	val, err = m.ds.Get(key)
	if err != nil {
		return err
	}
	err = ob.Deserialize(val)
	if err != nil {
		return err
	}
	of.base = ob
	of.prevEnd = ob.End

	ss := new(SeqState)
	key = store.NewKey(pb.MetaType_OrderSeqNumKey, m.localID, of.pro, ns.Nonce)
	val, err = m.ds.Get(key)
	if err != nil {
		return err
	}
	err = ss.Deserialize(val)
	if err != nil {
		return err
	}

	os := new(types.SignedOrderSeq)
	key = store.NewKey(pb.MetaType_OrderSeqKey, m.localID, of.pro, ns.Nonce, ss.Number)
	val, err = m.ds.Get(key)
	if err != nil {
		return err
	}
	err = os.Deserialize(val)
	if err != nil {
		return err
	}

	of.seq = os
	of.seqState = ss.State
	of.seqTime = ss.Time
	of.seqNum = ss.Number + 1

	if os.Size > of.base.Size {
		of.base.Size = os.Size
		of.base.Price.Set(os.Price)
	}

	key = store.NewKey(pb.MetaType_OrderSeqJobKey, m.localID, of.pro, ns.Nonce, ss.Number)
	val, err = m.ds.Get(key)
	if err != nil {
		return err
	}
	err = of.sjq.Deserialize(val)
	if err != nil {
		return err
	}

	return nil
}

func (m *OrderMgr) startProInst(of *proInst) {
	go m.update(of.pro)

	// send chunks via net
	go m.sendChunk(of)
	// change order state
	go m.runOrderSched(of)
}
