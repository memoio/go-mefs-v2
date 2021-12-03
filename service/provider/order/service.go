package order

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"math/big"
	"sync"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

type OrderMgr struct {
	sync.RWMutex

	api.IRole
	api.INetService
	api.IDataService
	api.IState

	ctx context.Context
	ds  store.KVStore

	localID uint64
	quo     *types.Quotation

	orders map[uint64]*OrderFull // key: userID
}

func NewOrderMgr(ctx context.Context, roleID uint64, ds store.KVStore, ir api.IRole, in api.INetService, id api.IDataService, is api.IState) *OrderMgr {
	quo := &types.Quotation{
		ProID:      roleID,
		TokenIndex: 1,
		SegPrice:   new(big.Int).Set(build.DefaultSegPrice),
		PiecePrice: new(big.Int).Set(build.DefaultPiecePrice),
	}

	om := &OrderMgr{
		IRole:        ir,
		IDataService: id,
		INetService:  in,
		IState:       is,

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

		or.dv.Add(id, data, tags[0])

		//or.seq.DataName = append(or.seq.DataName, id)
		as := &types.AggSegs{
			BucketID: seg.SegmentID().GetBucketID(),
			Start:    seg.SegmentID().GetStripeID(),
			Length:   1,
			ChunkID:  seg.SegmentID().GetChunkID(),
		}

		or.seq.Segments.Push(as)
		// update size and price
		or.seq.Price.Add(or.seq.Price, or.segPrice)
		or.seq.Size += build.DefaultSegSize

		or.seq.Segments.Merge()

		data, err := or.seq.Serialize()
		if err != nil {
			return err
		}

		key := store.NewKey(pb.MetaType_OrderSeqKey, m.localID, or.base.UserID, or.base.Nonce, or.seq.SeqNum)
		err = m.ds.Put(key, data)
		if err != nil {
			return err
		}

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

	return data, nil
}

func (m *OrderMgr) HandleCreateOrder(b []byte) ([]byte, error) {
	ob := new(types.SignedOrder)
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

			key := store.NewKey(pb.MetaType_OrderNonceKey, m.localID, or.base.UserID)
			ns := &NonceState{
				Nonce: or.base.Nonce,
				Time:  or.orderTime,
				State: or.orderState,
			}
			val, err := cbor.Marshal(ns)
			if err != nil {
				return nil, err
			}
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

			or.segPrice = new(big.Int).Mul(ob.SegPrice, big.NewInt(build.DefaultSegSize))

			// reset seq
			or.seqNum = 0
			or.seqState = OrderSeq_Init

			// reset data verifier
			or.dv.Reset()

			psig, err := m.RoleSign(m.ctx, m.localID, or.base.Hash(), types.SigSecp256k1)
			if err != nil {
				return nil, err
			}
			or.base.Psign = psig

			// save order base
			key := store.NewKey(pb.MetaType_OrderBaseKey, m.localID, or.base.UserID, or.base.Nonce)
			data, err := or.base.Serialize()
			if err != nil {
				return nil, err
			}
			err = m.ds.Put(key, data)
			if err != nil {
				return nil, err
			}

			// save state
			key = store.NewKey(pb.MetaType_OrderNonceKey, m.localID, or.base.UserID)
			ns := &NonceState{
				Nonce: or.base.Nonce,
				Time:  or.orderTime,
				State: or.orderState,
			}
			val, err := cbor.Marshal(ns)
			if err != nil {
				return nil, err
			}
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
	os := new(types.SignedOrderSeq)
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
		// verify accPrice and accSize
		if os.Price.Cmp(or.base.Price) != 0 || os.Size != or.base.Size {
			logger.Debug("handle receive:", os.Price, os.Size)
			logger.Debug("handle local:", or.base.Price, or.base.Size)
		}

		or.seq = os
		or.seqState = OrderSeq_Ack
		or.seqTime = time.Now().Unix()
		or.seqNum++

		// save seq
		key := store.NewKey(pb.MetaType_OrderSeqKey, m.localID, or.base.UserID, or.base.Nonce, or.seq.SeqNum)
		data, err := or.seq.Serialize()
		if err != nil {
			return nil, err
		}
		err = m.ds.Put(key, data)
		if err != nil {
			return nil, err
		}

		// save seq state
		key = store.NewKey(pb.MetaType_OrderSeqNumKey, m.localID, userID, or.base.Nonce)
		ss := SeqState{
			Number: or.seq.SeqNum,
			Time:   or.seqTime,
			State:  or.seqState,
		}
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
	os := new(types.SignedOrderSeq)
	err := os.Deserialize(b)
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
			or.seq.Segments.Merge()

			// compare local and remote
			rHash, err := os.Hash()
			if err != nil {
				return nil, err
			}
			lHash, err := or.seq.Hash()
			if err != nil {
				return nil, err
			}
			if !bytes.Equal(lHash[:], rHash[:]) {
				logger.Debug("handle seq local:", or.seq.Segments.Len(), or.seq)
				logger.Debug("handle seq remote:", os.Segments.Len(), os)
				logger.Debug("handle seq md5:", hex.EncodeToString(lHash[:]), " and ", hex.EncodeToString(rHash[:]))

				// todo, load missing
				if !or.seq.Segments.Equal(os.Segments) {
					logger.Warn("segments are not equal, load or re-get missing")
					or.seq.Segments = os.Segments
					or.seq.Price = os.Price
					or.seq.Size = os.Size
					lHash = rHash
				}
			}

			ok, err := or.dv.Result()
			if err != nil {
				logger.Warn("data verify is wrong:", err)
			}
			if !ok {
				logger.Warn("data verify is wrong")
				//return nil, ErrDataSign
			}

			ok, err = m.RoleVerify(m.ctx, userID, lHash, os.UserDataSig)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, ErrDataSign
			}

			ssig, err := m.RoleSign(m.ctx, m.localID, lHash, types.SigSecp256k1)
			if err != nil {
				return nil, err
			}

			// add base
			or.base.Size = or.seq.Size
			or.base.Price.Set(or.seq.Price)
			sHash := or.base.Hash()

			ok, err = m.RoleVerify(m.ctx, userID, sHash, os.UserSig)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, ErrDataSign
			}

			osig, err := m.RoleSign(m.ctx, m.localID, sHash, types.SigSecp256k1)
			if err != nil {
				return nil, err
			}

			or.seq.UserDataSig = os.UserDataSig
			or.seq.UserSig = os.UserSig

			or.seq.ProDataSig = ssig
			or.seq.ProSig = osig

			or.seqState = OrderSeq_Done
			or.seqTime = time.Now().Unix()

			// save order seq
			key := store.NewKey(pb.MetaType_OrderSeqKey, m.localID, or.base.UserID, or.base.Nonce, os.SeqNum)
			data, err := or.seq.Serialize()
			if err != nil {
				return nil, err
			}
			err = m.ds.Put(key, data)
			if err != nil {
				return nil, err
			}

			// save seq state
			key = store.NewKey(pb.MetaType_OrderSeqNumKey, m.localID, userID, or.base.Nonce)
			ss := SeqState{
				Number: or.seq.SeqNum,
				Time:   or.seqTime,
				State:  or.seqState,
			}
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
