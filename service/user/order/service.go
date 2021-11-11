package order

import (
	"context"
	"encoding/binary"
	"sync"
	"time"

	"github.com/emirpasic/gods/sets/hashset"
	"github.com/fxamacker/cbor/v2"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

type OrderMgr struct {
	api.IRole
	api.INetService
	api.IDataService

	localID uint64
	fsID    []byte

	ds store.KVStore // save order info

	orders map[uint64]*OrderFull // key: proID

	proSet *hashset.Set             // all pros
	proMap map[uint64]*lastProvider // key: bucketID

	segs    []types.Segs
	segLock sync.Mutex

	newOrderChan chan uint64
	quoChan      chan *types.Quotation
	confirmChan  chan *types.OrderBase
	seqChan      chan *types.OrderSeq

	ready bool
}

func NewOrderMgr(ctx context.Context, roleID uint64, ds store.KVStore, ir api.IRole, id api.IDataService) *OrderMgr {
	proset := hashset.New()

	om := &OrderMgr{
		IRole:        ir,
		IDataService: id,
		proSet:       proset,

		newOrderChan: make(chan uint64, 16),
		quoChan:      make(chan *types.Quotation, 16),
		confirmChan:  make(chan *types.OrderBase, 16),
		seqChan:      make(chan *types.OrderSeq, 16),
	}

	om.load(ctx)
	om.addPros()

	om.ready = true

	go om.runSched(ctx)

	return om
}

func (m *OrderMgr) Ready() bool {
	return m.ready
}

func (m *OrderMgr) load(ctx context.Context) error {
	key := store.NewKey(pb.MetaType_OrderProsKey, m.localID)
	val, err := m.ds.Get(key)
	if err != nil {
		return err
	}
	for i := 0; i < len(val)/8; i++ {
		pid := binary.BigEndian.Uint64(val[8*i : 8*(i+1)])
		m.proSet.Add(pid)
		m.newOrder(pid)
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
			m.newOrder(pro)
			m.proSet.Add(pro)
		}
	}
	// check connect
}

func (m *OrderMgr) removePro() {
	// remove it if not connected for a long time
}

func (m *OrderMgr) runSched(ctx context.Context) {
	st := time.NewTicker(time.Minute)
	defer st.Stop()

	lt := time.NewTicker(time.Minute)
	defer lt.Stop()
	for {
		select {
		case <-st.C:
		case <-lt.C:
			m.addPros() // add providers

			go m.save(ctx)
			// handerRunning?
			// handleDone
		case ob := <-m.confirmChan:
			m.confirmOrderBase(ob)
		case <-m.seqChan:
			// todo
		case <-ctx.Done():
			return
		}
	}
}

func (m *OrderMgr) getQuotation(ctx context.Context, id uint64) error {
	resp, err := m.SendMetaRequest(ctx, id, pb.NetMessage_AskPrice, nil, nil)
	if err != nil {
		return err
	}

	quo := new(types.Quotation)
	err = cbor.Unmarshal(resp.GetData().GetMsgInfo(), quo)
	if err != nil {
		return err
	}

	m.quoChan <- quo

	return nil
}

// confirm base
func (m *OrderMgr) confirmOrderBase(ob *types.OrderBase) error {
	// state:->confirm
	or, ok := m.orders[ob.ProID]
	if !ok {
		return ErrState
	}

	or.run()

	return nil
}

// handle ack pro pro

// receive order ack; Order_Init -> Order_Running
func (m *OrderMgr) handleOrderConfirm(ob *types.OrderBase) {
	// verify sign; persist
	m.confirmChan <- ob
}

// receive seq ack; OrderSeq_Running -> OrderSeq_Sending
func (m *OrderMgr) handleSeqConfirm(s *types.OrderSeq) {
	m.seqChan <- s
}

// receive seq done ack;OrderSeq_Sending -> OrderSeq_Done
func (m *OrderMgr) handleSeqDoneConfirm(s *types.OrderSeq) {
	m.seqChan <- s
}

// get information
