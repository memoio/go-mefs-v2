package order

import (
	"context"
	"encoding/binary"
	"math/big"
	"sync"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/memoio/go-mefs-v2/api"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	pdpv2 "github.com/memoio/go-mefs-v2/lib/crypto/pdp/version2"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/zeebo/blake3"
)

type OrderMgr struct {
	sync.RWMutex

	api.IRole
	api.INetService
	api.IDataService

	ctx context.Context
	ds  store.KVStore

	localID uint64
	quo     *types.Quotation

	orders map[uint64]*OrderFull // key: userID
}

func NewOrderMgr(ctx context.Context, roleID uint64, ds store.KVStore, ir api.IRole, in api.INetService, id api.IDataService) *OrderMgr {
	quo := &types.Quotation{
		ProID:      roleID,
		TokenIndex: 1,
		SegPrice:   big.NewInt(1234),
		PiecePrice: big.NewInt(5678),
	}

	om := &OrderMgr{
		IRole:        ir,
		IDataService: id,
		INetService:  in,

		ctx: ctx,
		ds:  ds,

		localID: roleID,
		quo:     quo,

		orders: make(map[uint64]*OrderFull),
	}

	om.load()

	return om

}

func (m *OrderMgr) load() error {
	key := store.NewKey(pb.MetaType_OrderUsersKey, m.localID)
	val, err := m.ds.Get(key)
	if err != nil {
		return err
	}

	for i := 0; i < len(val)/8; i++ {
		pid := binary.BigEndian.Uint64(val[8*i : 8*(i+1)])
		m.orders[pid] = m.loadOrder(pid)
	}

	pros, _ := m.IRole.RoleGetRelated(m.ctx, pb.RoleInfo_Provider)
	for _, pid := range pros {
		has := false
		for pro := range m.orders {
			if pid == pro {
				has = true
			}
		}

		if !has {
			m.orders[pid] = m.loadOrder(pid)
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

	key := store.NewKey(pb.MetaType_OrderUsersKey, m.localID)
	return m.ds.Put(key, buf)
}

func (m *OrderMgr) runSched(ctx context.Context) {
	st := time.NewTicker(time.Minute)
	defer st.Stop()

	for {
		select {
		case <-st.C:
		case <-ctx.Done():
			return
		}
	}
}

func (m *OrderMgr) HandleData(userID uint64, seg segment.Segment) error {
	m.RLock()
	or, ok := m.orders[userID]
	m.RUnlock()
	if !ok {
		or = m.loadOrder(userID)

		m.Lock()
		m.orders[userID] = or
		m.Unlock()
	}

	if or.base == nil || or.seq == nil {
		return ErrState
	}

	logger.Debug("handle: ", or.nonce, or.seqNum, or.orderState, or.seqState)

	if or.orderState == Order_Ack && or.seqState == OrderSeq_Ack {
		id := seg.SegmentID().Bytes()
		data, _ := seg.Content()
		tags, _ := seg.Tags()

		or.dv.Input(seg.SegmentID().Bytes(), data, tags[0])

		or.seq.DataName = append(or.seq.DataName, id)
		// update size and price

		return nil
	}

	return ErrState
}

func (m *OrderMgr) HandleQuotation(userID uint64) ([]byte, error) {
	m.RLock()
	defer m.RUnlock()
	data, err := cbor.Marshal(m.quo)
	if err != nil {
		return nil, err
	}

	go m.getBlsPubkey(userID)

	return data, nil
}

func (m *OrderMgr) HandleCreateOrder(b []byte) ([]byte, error) {
	ob := new(types.OrderBase)
	err := cbor.Unmarshal(b, ob)
	if err != nil {
		return nil, err
	}

	m.RLock()
	or, ok := m.orders[ob.UserID]
	m.RUnlock()
	if !ok {
		or = m.loadOrder(ob.UserID)
		m.createOrder(or)

		m.Lock()
		m.orders[ob.UserID] = or
		m.Unlock()
	}

	if !or.ready {
		go m.createOrder(or)
		return nil, ErrService
	}

	logger.Debug("handle: ", or.nonce, or.seqNum, or.orderState, or.seqState)
	if or.nonce == ob.Nonce {
		// handle previous
		if or.base != nil && or.orderState == Order_Ack {
			or.orderState = Order_Done
			or.orderTime = time.Now().Unix()
			ns := &NonceState{
				Nonce: or.base.Nonce,
				Time:  or.orderTime,
				State: or.orderState,
			}

			val, err := cbor.Marshal(ns)
			if err != nil {
				return nil, err
			}

			key := store.NewKey(pb.MetaType_OrderNonceKey, m.localID, or.base.UserID)
			err = m.ds.Put(key, val)
			if err != nil {
				return nil, err
			}

			or.base = nil
			or.orderState = Order_Init
		}

		if or.orderState == Order_Done {
			or.orderState = Order_Init
		}

		if or.orderState == Order_Init {
			or.base = ob
			or.orderState = Order_Ack
			or.orderTime = time.Now().Unix()
			or.nonce++

			// reset data verifier
			or.dv.Reset()

			data, err := or.base.Serialize()
			if err != nil {
				return nil, err
			}

			key := store.NewKey(pb.MetaType_OrderBaseKey, m.localID, or.base.UserID, or.base.Nonce)
			err = m.ds.Put(key, data)
			if err != nil {
				return nil, err
			}

			// save state
			ns := &NonceState{
				Nonce: or.base.Nonce,
				Time:  or.orderTime,
				State: or.orderState,
			}

			val, err := cbor.Marshal(ns)
			if err != nil {
				return nil, err
			}

			key = store.NewKey(pb.MetaType_OrderNonceKey, m.localID, or.base.UserID)
			err = m.ds.Put(key, val)
			if err != nil {
				return nil, err
			}

			return data, nil
		}
	}

	// been acked
	if or.base != nil && or.base.Nonce == ob.Nonce && or.orderState == Order_Ack {
		data, err := or.base.Serialize()
		if err != nil {
			return nil, err
		}

		return data, nil
	}

	return nil, ErrState
}

func (m *OrderMgr) HandleCreateSeq(userID uint64, b []byte) ([]byte, error) {
	os := new(types.OrderSeq)
	err := cbor.Unmarshal(b, os)
	if err != nil {
		return nil, err
	}

	m.RLock()
	or, ok := m.orders[userID]
	m.RUnlock()
	if !ok {
		or = m.loadOrder(userID)

		m.Lock()
		m.orders[userID] = or
		m.Unlock()
	}

	if !or.ready {
		go m.createOrder(or)
		return nil, ErrService
	}

	logger.Debug("handle: ", or.nonce, or.seqNum, or.orderState, or.seqState)

	if or.base == nil || or.orderState != Order_Ack {
		return nil, ErrState
	}

	if or.seqNum == os.SeqNum && (or.seqState == OrderSeq_Init || or.seqState == OrderSeq_Done) {
		or.seq = os
		or.seqState = OrderSeq_Ack
		or.seqTime = time.Now().Unix()
		or.seqNum++

		data, err := or.seq.Serialize()
		if err != nil {
			return nil, err
		}

		key := store.NewKey(pb.MetaType_OrderSeqKey, m.localID, or.base.UserID, or.base.Nonce, or.seq.SeqNum)
		err = m.ds.Put(key, data)
		if err != nil {
			return nil, err
		}

		ss := SeqState{
			Number: or.seq.SeqNum,
			Time:   or.seqTime,
			State:  or.seqState,
		}
		key = store.NewKey(pb.MetaType_OrderSeqNumKey, m.localID, userID, or.base.Nonce)
		val, err := cbor.Marshal(ss)
		if err != nil {
			return nil, err
		}

		err = m.ds.Put(key, val)
		if err != nil {
			return nil, err
		}

		return data, nil
	}

	// been acked
	if or.seq != nil && or.seq.SeqNum == os.SeqNum && or.seqState == OrderSeq_Ack {
		data, err := or.seq.Serialize()
		if err != nil {
			return nil, err
		}
		return data, nil
	}

	return nil, ErrState
}

func (m *OrderMgr) HandleFinishSeq(userID uint64, b []byte) ([]byte, error) {
	os := new(types.OrderSeq)
	err := cbor.Unmarshal(b, os)
	if err != nil {
		return nil, err
	}

	m.RLock()
	or, ok := m.orders[userID]
	m.RUnlock()
	if !ok {
		or = m.loadOrder(userID)

		m.Lock()
		m.orders[userID] = or
		m.Unlock()
	}

	if !or.ready {
		go m.createOrder(or)
		return nil, ErrService
	}

	logger.Debug("handle: ", or.nonce, or.seqNum, or.orderState, or.seqState)

	if or.seq != nil && or.seq.SeqNum == os.SeqNum {
		if or.seqState == OrderSeq_Ack {
			ok := or.dv.Result()
			if !ok {
				return nil, ErrSign
			}

			or.seqState = OrderSeq_Done
			or.seqTime = time.Now().Unix()
			data, err := or.seq.Serialize()
			if err != nil {
				return nil, err
			}

			key := store.NewKey(pb.MetaType_OrderSeqKey, m.localID, or.base.UserID, or.base.Nonce, os.SeqNum)
			err = m.ds.Put(key, data)
			if err != nil {
				return nil, err
			}

			ss := SeqState{
				Number: or.seq.SeqNum,
				Time:   or.seqTime,
				State:  or.seqState,
			}
			key = store.NewKey(pb.MetaType_OrderSeqNumKey, m.localID, userID, or.base.Nonce)
			val, err := cbor.Marshal(ss)
			if err != nil {
				return nil, err
			}
			err = m.ds.Put(key, val)
			if err != nil {
				return nil, err
			}

			return data, nil
		}

		if or.seqState == OrderSeq_Done {
			data, err := or.seq.Serialize()
			if err != nil {
				return nil, err
			}

			return data, nil
		}
	}

	return nil, ErrState
}

// need retry?
func (m *OrderMgr) getBlsPubkey(userID uint64) (pdpcommon.PublicKey, error) {
	// get bls publickey from local
	logger.Debug("get pdp publickey for: ", userID)
	key := store.NewKey(userID, pb.MetaType_PDPProveKey)
	pk := new(pdpv2.PublicKey)

	val, err := m.ds.Get(key)
	if err == nil {
		err = pk.Deserialize(val)
		if err == nil {
			logger.Debug("get pdp publickey local for: ", userID)
			return pk, nil
		}
	}

	// get from remote
	resp, err := m.SendMetaRequest(m.ctx, userID, pb.NetMessage_Get, key, nil)
	if err != nil {
		logger.Debug("fail get pdp publickey for: ", userID, err)
		return pk, err
	}

	sig := new(types.Signature)
	err = sig.Deserialize(resp.GetData().GetSign())
	if err != nil {
		logger.Debug("fail get pdp publickey for: ", userID, err)
		return pk, err
	}

	data := resp.GetData().GetMsgInfo()
	err = pk.Deserialize(data)
	if err != nil {
		logger.Debug("fail get pdp publickey for: ", userID, err)
		return pk, err
	}

	msg := blake3.Sum256(data)
	ok, _ := m.RoleVerify(m.ctx, userID, msg[:], *sig)
	if ok {
		return pk, nil
	}

	return pk, ErrSign
}
