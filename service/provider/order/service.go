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

	users  []uint64
	orders map[uint64]*OrderFull // key: userID
}

func NewOrderMgr(ctx context.Context, roleID uint64, ds store.KVStore, ir api.IRole, in api.INetService, id api.IDataService, pp api.IChainState, scm api.ISettle) *OrderMgr {
	quo := &types.Quotation{
		ProID:      roleID,
		TokenIndex: 0,
		SegPrice:   new(big.Int).Set(build.DefaultSegPrice),
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

		users:  make([]uint64, 0, 128),
		orders: make(map[uint64]*OrderFull),
	}

	return om
}

func (m *OrderMgr) Start() {
	// load some
	users := m.ics.StateGetUsersAt(m.ctx, m.localID)
	for _, uid := range users {
		m.users = append(m.users, uid)
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
		or.seq.Price.Add(or.seq.Price, or.segPrice)
		or.seq.Size += build.DefaultSegSize

		saveOrderSeq(or, m.ds)

		go func() {
			id := seg.SegmentID().Bytes()
			data, _ := seg.Content()
			tags, _ := seg.Tags()

			or.dv.Add(id, data, tags[0])
		}()

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

	err = lib.CheckOrder(ob.OrderBase)
	if err != nil {
		return nil, err
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

	logger.Debug("handle create order: ", ob.UserID, or.nonce, or.seqNum, or.orderState, or.seqState)
	if or.nonce == ob.Nonce {
		// handle previous
		if or.base != nil && or.orderState == Order_Ack {
			or.orderState = Order_Done
			or.orderTime = time.Now().Unix()

			err = saveOrderState(or, m.ds)
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
				or.nonce++

				or.segPrice = new(big.Int).Mul(ob.SegPrice, big.NewInt(build.DefaultSegSize))

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

	logger.Debug("handle create seq: ", userID, or.nonce, or.seqNum, or.orderState, or.seqState)

	if or.base == nil || or.orderState != Order_Ack {
		return nil, xerrors.Errorf("fail create seq user %d nonce %d seq %d state %s %s, got %d %d", or.userID, or.nonce, or.seqNum, or.orderState, or.seqState, os.Nonce, os.SeqNum)
	}

	if or.seqNum == os.SeqNum && (or.seqState == OrderSeq_Init || or.seqState == OrderSeq_Done) {
		// verify accPrice and accSize
		if os.Price.Cmp(or.base.Price) != 0 || os.Size != or.base.Size {
			logger.Debug("handle seq receive:", os.Price, os.Size)
			logger.Debug("handle seq local:", or.base.Price, or.base.Size)
			return nil, xerrors.Errorf("fail create seq due to wrong size")
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

	logger.Debug("handle finish seq: ", userID, or.nonce, or.seqNum, or.orderState, or.seqState)

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

							id := sid.Bytes()
							data, _ := segmt.Content()
							tags, _ := segmt.Tags()

							or.dv.Add(id, data, tags[0])
						}
					}
					or.seq.Segments = os.Segments

					// need verify again
					or.seq.Price = os.Price
					or.seq.Size = os.Size
					lHash = rHash
					//return nil, xerrors.Errorf("segments are not equal, load or re-get missing")
				}
			}

			ok, err := or.dv.Result()
			if err != nil {
				logger.Warn("data verify is wrong:", err)
				return nil, err
			}
			if !ok {
				logger.Warn("data verify is wrong")
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

			// save order seq

			err = saveOrderSeq(or, m.ds)
			if err != nil {
				return nil, err
			}

			err = saveSeqState(or, m.ds)
			if err != nil {
				return nil, err
			}

			return or.seq.Serialize()
		}

		if or.seqState == OrderSeq_Done {
			if !os.Hash().Equal(or.seq.Hash()) {
				logger.Debug("handle seq local:", or.seq.Segments.Len(), or.seq)
				logger.Debug("handle seq remote:", os.Segments.Len(), os)
				return nil, xerrors.Errorf("segments are not equal, load or re-get missing")
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
