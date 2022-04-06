package order

import (
	"bytes"
	"context"
	"math/big"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

type OrderMgr struct {
	ir  api.IRole
	ids api.IDataService
	ics api.IChainState
	ins api.INetService
	is  api.ISettle
	ds  store.KVStore

	lk  sync.RWMutex
	ctx context.Context

	localID uint64
	quo     *types.Quotation

	di *DataInfo

	users  []uint64
	orders map[uint64]*OrderFull // key: userID
}

func NewOrderMgr(ctx context.Context, roleID uint64, price uint64, ds store.KVStore, ir api.IRole, in api.INetService, id api.IDataService, pp api.IChainState, scm api.ISettle) *OrderMgr {
	quo := &types.Quotation{
		ProID:      roleID,
		TokenIndex: 0,
		SegPrice:   new(big.Int).SetUint64(price),
		PiecePrice: new(big.Int).Set(build.DefaultPiecePrice),
	}

	om := &OrderMgr{
		ir:  ir,
		ids: id,

		ics: pp,
		ins: in,
		is:  scm,

		ctx: ctx,
		ds:  ds,

		localID: roleID,
		quo:     quo,

		di: &DataInfo{
			OrderPayInfo: types.OrderPayInfo{
				NeedPay: big.NewInt(0),
				Paid:    big.NewInt(0),
				Balance: big.NewInt(0),
			},
		},

		users:  make([]uint64, 0, 128),
		orders: make(map[uint64]*OrderFull),
	}

	return om
}

func (m *OrderMgr) Start() {
	// load data info
	key := store.NewKey(pb.MetaType_OrderPayInfoKey, m.localID)
	val, err := m.ds.Get(key)
	if err == nil {
		m.di.Deserialize(val)
	}
	// load some
	users := m.ics.StateGetUsersAt(m.ctx, m.localID)
	for _, uid := range users {
		m.getOrder(uid)
	}

	go m.runCheck()
}

func (m *OrderMgr) HandleData(userID uint64, seg segment.Segment) error {
	or := m.getOrder(userID)

	or.lw.Lock()
	defer or.lw.Unlock()

	if !or.ready {
		return xerrors.Errorf("order service not ready for %d", userID)
	}

	if or.base == nil {
		return xerrors.Errorf("no order base for %d", userID)
	}

	if or.seq == nil {
		return xerrors.Errorf("no order seq for %d", userID)
	}

	if or.seq.Size-or.base.Size > seqMaxSize {
		return xerrors.Errorf("%d order %d seq %d sub size %d %d is larger than %d", userID, or.seq.Nonce, or.seq.SeqNum, or.seq.Size, or.base.Size, seqMaxSize)
	}

	or.availTime = time.Now().Unix()

	logger.Debug("handle add data: ", userID, or.nonce, or.seqNum, or.orderState, or.seqState, seg.SegmentID())

	if or.orderState == Order_Ack && or.seqState == OrderSeq_Ack {
		if or.seq.Segments.Has(seg.SegmentID().GetBucketID(), seg.SegmentID().GetStripeID(), seg.SegmentID().GetChunkID()) {
			logger.Debug("handle add data already: ", userID, or.nonce, or.seqNum, or.orderState, or.seqState, seg.SegmentID())
		}

		has, _ := m.ids.HasSegment(m.ctx, seg.SegmentID())
		if has {
			logger.Debug("handle add data already: ", userID, or.nonce, or.seqNum, or.orderState, or.seqState, seg.SegmentID())
			return nil
		}

		// put to local when received
		err := m.ids.PutSegmentToLocal(m.ctx, seg)
		if err != nil {
			return err
		}

		as := &types.AggSegs{
			BucketID: seg.SegmentID().GetBucketID(),
			Start:    seg.SegmentID().GetStripeID(),
			Length:   1,
			ChunkID:  seg.SegmentID().GetChunkID(),
		}
		or.seq.Segments.Push(as)
		or.seq.Segments.Merge()

		// update size and price
		or.seq.Price.Add(or.seq.Price, or.base.SegPrice)
		or.seq.Size += build.DefaultSegSize

		saveOrderSeq(or, m.ds)

		or.di.Received += build.DefaultSegSize
		key := store.NewKey(pb.MetaType_OrderPayInfoKey, m.localID, or.userID)
		val, _ := or.di.Serialize()
		m.ds.Put(key, val)

		go func(nseg segment.Segment) {

			id := nseg.SegmentID().Bytes()
			data, _ := nseg.Content()
			tags, _ := nseg.Tags()

			or.lw.Lock()
			defer or.lw.Unlock()
			or.dv.Add(id, data, tags[0])
		}(seg)

		return nil
	}

	return xerrors.Errorf("fail handle data user %d nonce %d seq %d state %s %s", userID, or.nonce, or.seqNum, or.orderState, or.seqState)
}

func (m *OrderMgr) HandleQuotation(userID uint64) ([]byte, error) {
	m.lk.RLock()
	defer m.lk.RUnlock()
	data, err := m.quo.Serialize()
	if err != nil {
		return nil, err
	}

	return data, nil
}

// todo: verify nonce in data&settle chain
func (m *OrderMgr) HandleCreateOrder(b []byte) ([]byte, error) {
	ob := new(types.SignedOrder)
	err := ob.Deserialize(b)
	if err != nil {
		return nil, err
	}

	sgi, err := m.ics.StateGetInfo(m.ctx)
	if err != nil {
		return nil, err
	}

	orderMin := build.OrderDurMap[sgi.Version]
	nht, ok := build.UpdateMap[sgi.Version+1]
	if ok {
		if nht-sgi.Height < 2880 {
			orderMin = build.OrderDurMap[sgi.Version+1]
		}
	}

	if ob.OrderBase.End-ob.OrderBase.Start < orderMin {
		return nil, xerrors.Errorf("order duration %d is short than %d", ob.OrderBase.End-ob.OrderBase.Start, orderMin)
	}

	err = lib.CheckOrder(ob.OrderBase)
	if err != nil {
		return nil, err
	}

	if ob.TokenIndex != m.quo.TokenIndex {
		return nil, xerrors.Errorf("seg token index is wrong, expected %d, got %d", m.quo.TokenIndex, ob.TokenIndex)
	}

	if ob.SegPrice.Cmp(m.quo.SegPrice) < 0 {
		return nil, xerrors.Errorf("seg price is lower than expected %d, got %d", m.quo.SegPrice, ob.SegPrice)
	}

	nt := time.Now().Unix()
	if ob.Start < nt && nt-ob.Start > types.Hour {
		return nil, xerrors.Errorf("order start %d is far from %d", ob.Start, nt)

	} else if ob.Start > nt && ob.Start-nt > types.Hour {
		return nil, xerrors.Errorf("order start %d is far from %d", ob.Start, nt)
	}

	or := m.getOrder(ob.UserID)
	or.lw.Lock()
	defer or.lw.Unlock()

	if !or.ready {
		go m.createOrder(or)
		return nil, xerrors.Errorf("order service not ready for %d", ob.UserID)
	}

	if or.pause {
		return nil, xerrors.Errorf("order service pause for %d", ob.UserID)
	}

	or.availTime = time.Now().Unix()

	logger.Debug("handle create order: ", ob.UserID, ob.Nonce, or.nonce, or.seqNum, or.orderState, or.seqState)
	if or.nonce == ob.Nonce {
		// handle previous
		if or.base != nil && or.orderState == Order_Ack {
			or.orderState = Order_Done
			or.orderTime = time.Now().Unix()

			err = saveOrderState(or, m.ds)
			if err != nil {
				return nil, err
			}

			if or.base.Size == 0 {
				return nil, xerrors.Errorf("not create order due to prevous order %d is empty", or.base.Nonce)
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
			or.nonce = ob.Nonce + 1

			// reset seq
			or.seqNum = 0
			or.seqState = OrderSeq_Init

			// reset data verifier
			or.dv.Reset()

			psig, err := m.ir.RoleSign(m.ctx, m.localID, or.base.Hash(), types.SigSecp256k1)
			if err != nil {
				return nil, err
			}
			or.base.Psign = psig

			// save order base
			err = saveOrderBase(or, m.ds)
			if err != nil {
				return nil, err
			}

			// save order state
			err = saveOrderState(or, m.ds)
			if err != nil {
				return nil, err
			}

			return or.base.Serialize()
		}
	}

	// been acked
	if or.base != nil && or.base.Nonce == ob.Nonce && or.orderState == Order_Ack {
		if bytes.Equal(ob.Hash(), or.base.Hash()) {
			data, err := or.base.Serialize()
			if err != nil {
				return nil, err
			}

			return data, nil
		} else {
			// can replace old order if its size is zero
			if or.base.Size == 0 && (or.seq == nil || (or.seq != nil && or.seq.Size == 0)) {
				or.base = ob
				or.orderState = Order_Ack
				or.orderTime = time.Now().Unix()
				or.nonce = ob.Nonce + 1

				// reset seq
				or.seqNum = 0
				or.seqState = OrderSeq_Init

				// reset data verifier
				or.dv.Reset()

				psig, err := m.ir.RoleSign(m.ctx, m.localID, or.base.Hash(), types.SigSecp256k1)
				if err != nil {
					return nil, err
				}
				or.base.Psign = psig

				// save order base
				err = saveOrderBase(or, m.ds)
				if err != nil {
					return nil, err
				}

				// save order state
				err = saveOrderState(or, m.ds)
				if err != nil {
					return nil, err
				}

				return or.base.Serialize()
			}
		}
	}

	return nil, xerrors.Errorf("fail create order user %d nonce %d seq %d state %s %s, got %d", or.userID, or.nonce, or.seqNum, or.orderState, or.seqState, ob.Nonce)
}

func (m *OrderMgr) HandleCreateSeq(userID uint64, b []byte) ([]byte, error) {
	os := new(types.SignedOrderSeq)
	err := os.Deserialize(b)
	if err != nil {
		return nil, err
	}

	or := m.getOrder(userID)
	or.lw.Lock()
	defer or.lw.Unlock()

	if !or.ready {
		return nil, xerrors.Errorf("order service not ready for %d", userID)
	}

	or.availTime = time.Now().Unix()

	logger.Debug("handle create seq: ", userID, os.Nonce, or.nonce, or.seqNum, or.orderState, or.seqState)

	if or.base == nil || or.orderState != Order_Ack || or.base.Nonce != os.Nonce {
		return nil, xerrors.Errorf("fail create seq user %d nonce %d seq %d state %s %s, got %d %d", or.userID, or.nonce, or.seqNum, or.orderState, or.seqState, os.Nonce, os.SeqNum)
	}

	if or.seqNum == os.SeqNum && (or.seqState == OrderSeq_Init || or.seqState == OrderSeq_Done) {
		// verify accPrice and accSize
		if os.Price.Cmp(or.base.Price) != 0 || os.Size != or.base.Size {
			logger.Debug("handle create seq: ", os.UserID, os.Nonce, os.SeqNum, os.Size, or.base.Size)
			return nil, xerrors.Errorf("fail create seq %d %d due to wrong size, got %d expect %d", os.Nonce, os.SeqNum, os.Size, or.base.Size)
		}

		// space time should not too large
		if or.base.Size > orderMaxSize {
			return nil, xerrors.Errorf("fail create seq %d %d due to large size, got %d sgould less than %d", os.Nonce, os.SeqNum, or.base.Size, orderMaxSize)
		}

		or.seq = os
		or.seqState = OrderSeq_Ack
		or.seqTime = time.Now().Unix()
		or.seqNum++

		// save seq
		err = saveOrderSeq(or, m.ds)
		if err != nil {
			return nil, err
		}

		// save seq state
		err = saveSeqState(or, m.ds)
		if err != nil {
			return nil, err
		}

		return or.seq.Serialize()
	}

	// been acked
	if or.seq != nil && or.seq.SeqNum == os.SeqNum && or.seqState == OrderSeq_Ack {
		data, err := or.seq.Serialize()
		if err != nil {
			return nil, err
		}
		return data, nil
	}

	return nil, xerrors.Errorf("fail create seq user %d nonce %d seq %d state %s %s, got %d %d", or.userID, or.nonce, or.seqNum, or.orderState, or.seqState, os.Nonce, os.SeqNum)
}

func (m *OrderMgr) HandleFinishSeq(userID uint64, b []byte) ([]byte, error) {
	os := new(types.SignedOrderSeq)
	err := os.Deserialize(b)
	if err != nil {
		return nil, err
	}

	or := m.getOrder(userID)

	or.lw.Lock()
	defer or.lw.Unlock()

	if !or.ready {
		return nil, xerrors.Errorf("order service not ready for %d", userID)
	}

	or.availTime = time.Now().Unix()

	logger.Debug("handle finish seq: ", userID, os.Nonce, or.nonce, or.seqNum, or.orderState, or.seqState)

	if or.base == nil || or.orderState != Order_Ack || or.base.Nonce != os.Nonce {
		return nil, xerrors.Errorf("fail finish seq user %d nonce %d seq %d state %s %s, got %d %d", or.userID, or.nonce, or.seqNum, or.orderState, or.seqState, os.Nonce, os.SeqNum)
	}

	if or.seq != nil && or.seq.SeqNum == os.SeqNum {
		if or.seqState == OrderSeq_Ack {
			or.seq.Segments.Merge()

			// compare local and remote
			rHash := os.Hash()
			lHash := or.seq.Hash()
			if !rHash.Equal(lHash) {
				logger.Debug("handle seq md5:", lHash.String(), " and ", rHash.String())

				// todo: load missing or reget
				if !or.seq.Segments.Equal(os.Segments) {
					logger.Debug("handle seq local:", or.seq.Segments.Len(), or.seq)
					logger.Debug("handle seq remote:", os.Segments.Len(), os)
					logger.Warn("segments are not equal, load or re-get missing")

					or.seq.Price.Set(or.base.Price)
					or.seq.Size = or.base.Size

					sid, err := segment.NewSegmentID(or.fsID, 0, 0, 0)
					if err != nil {
						return nil, err
					}

					or.dv.Reset()

					for _, seg := range os.Segments {
						sid.SetBucketID(seg.BucketID)
						for j := seg.Start; j < seg.Start+seg.Length; j++ {
							sid.SetStripeID(j)
							sid.SetChunkID(seg.ChunkID)
							segmt, err := m.ids.GetSegmentFromLocal(m.ctx, sid)
							if err != nil {
								// should not, how to fix this by user?
								return nil, err
							}

							id := segmt.SegmentID().Bytes()
							data, _ := segmt.Content()
							tags, _ := segmt.Tags()

							or.seq.Price.Add(or.seq.Price, or.base.SegPrice)
							or.seq.Size += build.DefaultSegSize

							or.dv.Add(id, data, tags[0])
						}
					}
					or.seq.Segments = os.Segments

					// need verify again
					lHash = or.seq.Hash()
					if !rHash.Equal(lHash) {
						return nil, xerrors.Errorf("segments are not equal, load or re-get missing")
					}
				}
			}

			ok, err := or.dv.Result()
			if err != nil {
				return nil, xerrors.Errorf("data verify fails %s", err)
			}
			if !ok {
				return nil, xerrors.Errorf("data verify is wrong")
			}

			ok, err = m.ir.RoleVerify(m.ctx, userID, lHash.Bytes(), os.UserDataSig)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, xerrors.Errorf("%d order seq sign is wrong", userID)
			}

			ssig, err := m.ir.RoleSign(m.ctx, m.localID, lHash.Bytes(), types.SigSecp256k1)
			if err != nil {
				return nil, err
			}

			// add base
			or.base.Size = or.seq.Size
			or.base.Price.Set(or.seq.Price)
			sHash := or.base.Hash()

			ok, err = m.ir.RoleVerify(m.ctx, userID, sHash, os.UserSig)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, xerrors.Errorf("%d order sign is wrong", userID)
			}

			osig, err := m.ir.RoleSign(m.ctx, m.localID, sHash, types.SigSecp256k1)
			if err != nil {
				return nil, err
			}

			or.seq.UserDataSig = os.UserDataSig
			or.seq.UserSig = os.UserSig

			or.seq.ProDataSig = ssig
			or.seq.ProSig = osig

			or.seqState = OrderSeq_Done
			or.seqTime = time.Now().Unix()

			or.di.Size += uint64(or.seq.Segments.Size()) * build.DefaultSegSize

			// save order seq

			err = saveOrderBase(or, m.ds)
			if err != nil {
				return nil, err
			}

			err = saveOrderSeq(or, m.ds)
			if err != nil {
				return nil, err
			}

			err = saveSeqState(or, m.ds)
			if err != nil {
				return nil, err
			}

			key := store.NewKey(pb.MetaType_OrderPayInfoKey, m.localID, or.userID)
			val, _ := or.di.Serialize()
			m.ds.Put(key, val)

			return or.seq.Serialize()
		}

		if or.seqState == OrderSeq_Done {
			if !os.Hash().Equal(or.seq.Hash()) {
				logger.Debug("handle seq local:", or.seq.Segments.Len(), or.seq)
				logger.Debug("handle seq remote:", os.Segments.Len(), os)
				return nil, xerrors.Errorf("done segments are not equal, load or re-get missing")
			}
			data, err := or.seq.Serialize()
			if err != nil {
				return nil, err
			}

			return data, nil
		}
	}

	return nil, xerrors.Errorf("fail finish seq user %d nonce %d seq %d state %s %s, got %d %d", or.userID, or.nonce, or.seqNum, or.orderState, or.seqState, os.Nonce, os.SeqNum)
}
