package challenge

import (
	"context"
	"encoding/binary"
	"math/big"
	"sync"
	"time"

	"github.com/zeebo/blake3"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/build"
	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12_381"
	"github.com/memoio/go-mefs-v2/lib/crypto/pdp"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/submodule/txPool"
)

type SegMgr struct {
	sync.RWMutex

	*txPool.PushPool

	api.IDataService

	ds store.KVStore

	ctx context.Context

	localID uint64

	epoch uint64
	eInfo *types.ChalEpoch

	users []uint64
	sInfo map[uint64]*segInfo // key: userID

	chalChan chan *chalRes

	inRemove bool
}

func NewSegMgr(ctx context.Context, localID uint64, ds store.KVStore, is api.IDataService, pp *txPool.PushPool) *SegMgr {
	s := &SegMgr{
		PushPool:     pp,
		IDataService: is,
		ctx:          ctx,
		localID:      localID,
		ds:           ds,
		users:        make([]uint64, 0, 128),
		sInfo:        make(map[uint64]*segInfo),

		chalChan: make(chan *chalRes, 16),
	}

	return s
}

func (s *SegMgr) Start() {
	s.load()
	go s.regularChallenge()
	go s.regularRemove()
}

// load from state
func (s *SegMgr) load() {
	users := s.StateGetUsersAt(s.ctx, s.localID)
	for _, pid := range users {
		s.loadFs(pid)
	}
}

func (s *SegMgr) AddUP(userID, proID uint64) {
	if proID != s.localID {
		return
	}

	logger.Debugf("add user %d for pro %d", userID, proID)

	go s.loadFs(userID)
}

func (s *SegMgr) loadFs(userID uint64) *segInfo {
	si, ok := s.sInfo[userID]
	if !ok {
		pk, err := s.StateGetPDPPublicKey(s.ctx, userID)
		if err != nil {
			logger.Debug("challenge not get fs")
			return nil
		}

		// load from local
		si = &segInfo{
			userID: userID,
			//pk:     pk,
			fsID: pk.VerifyKey().Hash(),
		}

		s.users = append(s.users, userID)
		s.sInfo[userID] = si
	}

	return si
}

func (s *SegMgr) regularChallenge() {
	tc := time.NewTicker(time.Minute)
	defer tc.Stop()

	s.epoch = s.GetChalEpoch(s.ctx)
	s.eInfo, _ = s.StateGetChalEpochInfo(s.ctx)

	i := 0
	for {
		select {
		case <-s.ctx.Done():
			return
		case ch := <-s.chalChan:
			si := s.loadFs(ch.userID)
			if ch.errCode != 0 {
				si.chalTime = time.Now()
				si.wait = false
				continue
			}

			si.chalTime = time.Now()
			si.wait = false
			si.nextChal++
		case <-tc.C:
			s.epoch = s.GetChalEpoch(s.ctx)
			s.eInfo, _ = s.StateGetChalEpochInfo(s.ctx)
			logger.Debug("challenge update epoch: ", s.eInfo.Epoch, s.epoch)
		default:
			if len(s.users) == 0 {
				logger.Debug("challenge no users")
				time.Sleep(10 * time.Second)
				continue
			}

			if len(s.users) <= i {
				i = 0
				time.Sleep(10 * time.Second)
			}

			userID := s.users[i]
			s.challenge(userID)
			i++
		}
	}
}

func (s *SegMgr) regularRemove() {
	tc := time.NewTicker(time.Minute)
	defer tc.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-tc.C:
			go s.removeExpiredChunk()
		}
	}
}

func (s *SegMgr) removeExpiredChunk() {
	if s.inRemove {
		return
	}
	s.inRemove = true
	defer func() {
		s.inRemove = false
	}()

	users := s.StateGetUsersAt(s.ctx, s.localID)
	for _, userID := range users {
		subNonce := uint64(0)
		key := store.NewKey(pb.MetaType_OrderExpiredKey, userID, s.localID)
		val, err := s.ds.Get(key)
		if err == nil && len(val) >= 8 {
			subNonce = binary.BigEndian.Uint64(val)
		}

		ns := s.StateGetOrderState(s.ctx, userID, s.localID)

		for i := subNonce; i < ns.SubNonce; i++ {
			s.removeSegInExpiredOrder(userID, i)
		}
	}
}

// todo: add repair after check before challenge
// todo: should cancle when epoch passed
func (s *SegMgr) challenge(userID uint64) {
	logger.Debug("challenge for user: ", userID)
	if s.epoch < 2 {
		logger.Debug("challenge not at epoch: ", userID, s.epoch)
		return
	}
	si := s.loadFs(userID)
	if si.nextChal > s.eInfo.Epoch {
		logger.Debug("challenge at: ", userID, s.eInfo.Epoch)
		return
	} else {
		si.nextChal = s.eInfo.Epoch
	}

	if si.wait {
		if time.Since(si.chalTime) > 10*time.Minute {
			si.wait = false
			logger.Debug("challenge redo at: ", userID, si.nextChal)
		}
		return
	}

	if s.GetProof(userID, s.localID, si.nextChal) {
		logger.Debug("challenge has done: ", userID, si.nextChal)
		return
	}

	err := s.subDataOrder(userID)
	if err != nil {
		logger.Debug("challenge sub order fails")
	}

	// get epoch info
	nt := time.Now()

	buf := make([]byte, 8+len(s.eInfo.Seed.Bytes()))
	binary.BigEndian.PutUint64(buf[:8], userID)
	copy(buf[8:], s.eInfo.Seed.Bytes())
	bh := blake3.Sum256(buf)

	pk, err := s.StateGetPDPPublicKey(s.ctx, userID)
	if err != nil {
		logger.Debug("challenge not get fs")
		return
	}

	pchal, err := pdp.NewChallenge(pk.VerifyKey(), bh)
	if err != nil {
		logger.Debug("challenge cannot create chal: ", userID, si.nextChal)
		return
	}

	pf, err := pdp.NewProofAggregator(pk, bh)
	if err != nil {
		logger.Debug("challenge cannot create proof: ", userID, si.nextChal)
		return
	}

	sid, err := segment.NewSegmentID(si.fsID, 0, 0, 0)
	if err != nil {
		logger.Debug("challenge create seg fails: ", userID, si.nextChal, err)
		return
	}

	cnt := uint64(0)

	// challenge routine
	ns := s.GetOrderStateAt(userID, s.localID, si.nextChal)
	if ns.Nonce == 0 && ns.SeqNum == 0 {
		logger.Debug("challenge on empty data at epoch: ", userID, si.nextChal)
		return
	}

	pce, err := s.StateGetChalEpochInfoAt(s.ctx, si.nextChal-1)
	if err != nil {
		logger.Debug("challenge at wrong epoch: ", userID, si.nextChal, err)
		return
	}

	// calc size and price
	chalStart := build.BaseTime + int64(pce.Slot*build.SlotDuration)
	chalEnd := build.BaseTime + int64(s.eInfo.Slot*build.SlotDuration)
	chalDur := chalEnd - chalStart
	if chalDur <= 0 {
		logger.Debug("challenge at wrong epoch: ", userID, si.nextChal)
		return
	}

	//logger.Debug("challenge duration: ", chalStart, chalEnd, ns.Nonce, ns.SeqNum)

	orderDur := int64(0)
	price := new(big.Int)
	totalPrice := new(big.Int)
	totalSize := uint64(0)
	delSize := uint64(0)

	orderStart := ns.SubNonce
	orderEnd := ns.Nonce

	if ns.SeqNum > 0 {
		so, err := s.StateGetOrder(s.ctx, userID, s.localID, ns.Nonce)
		if err != nil {
			logger.Debug("challenge get order fails: ", userID, ns.Nonce, err)
			return
		}

		if so.Start >= chalEnd {
			logger.Debug("challenge not latest one, duo to time is not up")
		} else {
			if so.End <= chalStart {
				logger.Debug("challenge orders all expired: ", userID, si.nextChal, ns.Nonce)
				return
			}

			if so.Start <= chalStart && so.End >= chalEnd {
				orderDur = chalDur
			} else if so.Start <= chalStart && so.End <= chalEnd {
				orderDur = so.End - chalStart
			} else if so.Start >= chalStart && so.End >= chalEnd {
				orderDur = chalEnd - so.Start
			} else if so.Start >= chalStart && so.End <= chalEnd {
				orderDur = so.End - so.Start
			}

			for i := uint32(0); i < ns.SeqNum; i++ {
				sf, err := s.StateGetOrderSeq(s.ctx, userID, s.localID, ns.Nonce, i)
				if err != nil {
					logger.Debug("challenge get order seq fails: ", userID, ns.Nonce, i, err)
					return
				}

				// calc del price and size
				price.Set(sf.DelPart.Price)
				price.Mul(price, big.NewInt(orderDur))
				totalPrice.Sub(totalPrice, price)
				delSize += sf.DelPart.Size
				pchal.Add(bls.SubFr(sf.AccFr, sf.DelPart.AccFr))

				if i == ns.SeqNum-1 {
					totalSize += sf.Size
					price.Set(sf.Price)
					price.Mul(price, big.NewInt(orderDur))
					totalPrice.Add(totalPrice, price)
				}

				var fault types.AggSegsQueue
				for _, seg := range sf.Segments {
					sid.SetBucketID(seg.BucketID)
					for j := seg.Start; j < seg.Start+seg.Length; j++ {
						if sf.DelSegs.Has(seg.BucketID, j, seg.ChunkID) {
							logger.Debug("challenge skip fault seg: ", seg.BucketID, j, seg.ChunkID)
							continue
						}

						sid.SetStripeID(j)
						sid.SetChunkID(seg.ChunkID)
						segm, err := s.GetSegmentFromLocal(context.TODO(), sid)
						if err != nil {
							logger.Debug("challenge not have chunk for stripe: ", userID, sid)
							fault.Push(&types.AggSegs{
								BucketID: seg.BucketID,
								Start:    j,
								Length:   1,
								ChunkID:  seg.ChunkID,
							})
							continue
						}

						segData, _ := segm.Content()
						segTag, _ := segm.Tags()

						err = pf.Add(sid.Bytes(), segData, segTag[0])
						if err != nil {
							logger.Debug("challenge add to proof: ", userID, sid.ShortString(), err)
							return
						}
						cnt++
					}
				}

				if fault.Len() > 0 {
					fault.Merge()
					srp := &tx.SegRemoveParas{
						UserID:   si.userID,
						ProID:    s.localID,
						Nonce:    ns.Nonce,
						SeqNum:   i,
						Segments: fault,
					}

					data, err := srp.Serialize()
					if err != nil {
						return
					}

					msg := &tx.Message{
						Version: 0,
						From:    s.localID,
						To:      si.userID,
						Method:  tx.SegmentFault,
						Params:  data,
					}

					err = s.pushAndWaitMessage(msg)
					if err != nil {
						logger.Debug("challenge push remove seg msg fail: ", err)
					}
					return
				}
			}
		}
	}

	if ns.Nonce > 0 {
		// todo: choose some from [0, ns.Nonce)
		for i := ns.SubNonce; i < ns.Nonce; i++ {
			so, err := s.StateGetOrder(s.ctx, userID, s.localID, i)
			if err != nil {
				logger.Debug("challenge get order fails:", userID, i, err)
				return
			}

			if so.Start >= chalEnd {
				logger.Debug("challenge order is not up: ", userID, si.nextChal, i, so)
				continue
			}

			if so.End <= chalStart {
				logger.Debug("challenge order expired: ", userID, si.nextChal, i, so)
				orderStart = i + 1
				continue
			}

			if so.Start <= chalStart && so.End >= chalEnd {
				orderDur = chalDur
			} else if so.Start <= chalStart && so.End <= chalEnd {
				orderDur = so.End - chalStart
			} else if so.Start >= chalStart && so.End >= chalEnd {
				orderDur = chalEnd - so.Start
			} else if so.Start >= chalStart && so.End <= chalEnd {
				orderDur = so.End - so.Start
			}

			price.Set(so.Price)
			price.Sub(price, so.DelPart.Price)
			price.Mul(price, big.NewInt(orderDur))
			totalPrice.Add(totalPrice, price)
			delSize += so.DelPart.Size
			totalSize += so.Size

			pchal.Add(bls.SubFr(so.AccFr, so.DelPart.AccFr))

			for k := uint32(0); k < so.SeqNum; k++ {
				sf, err := s.StateGetOrderSeq(s.ctx, userID, s.localID, i, k)
				if err != nil {
					logger.Debug("challenge get order seq fails:", userID, i, k, err)
					return
				}

				var fault types.AggSegsQueue
				for _, seg := range sf.Segments {
					sid.SetBucketID(seg.BucketID)
					for j := seg.Start; j < seg.Start+seg.Length; j++ {
						if sf.DelSegs.Has(seg.BucketID, j, seg.ChunkID) {
							logger.Debug("challenge skip fault seg: ", seg.BucketID, j, seg.ChunkID)
							continue
						}

						sid.SetStripeID(j)
						sid.SetChunkID(seg.ChunkID)
						segm, err := s.GetSegmentFromLocal(context.TODO(), sid)
						if err != nil {
							logger.Debug("challenge not have chunk for stripe: ", userID, sid)
							fault.Push(&types.AggSegs{
								BucketID: seg.BucketID,
								Start:    j,
								Length:   1,
								ChunkID:  seg.ChunkID,
							})
							continue
						}

						segData, err := segm.Content()
						if err != nil {
							logger.Debug("challenge have fault chunk for stripe: ", userID, sid)
							fault.Push(&types.AggSegs{
								BucketID: seg.BucketID,
								Start:    j,
								Length:   1,
								ChunkID:  seg.ChunkID,
							})
							continue
						}
						segTag, err := segm.Tags()
						if err != nil || len(segTag) == 0 {
							logger.Debug("challenge have fault chunk for stripe: ", userID, sid)
							fault.Push(&types.AggSegs{
								BucketID: seg.BucketID,
								Start:    j,
								Length:   1,
								ChunkID:  seg.ChunkID,
							})
							continue
						}

						err = pf.Add(sid.Bytes(), segData, segTag[0])
						if err != nil {
							logger.Debug("challenge add to proof: ", userID, sid.ShortString(), err)
							return
						}
						cnt++
					}
				}

				if fault.Len() > 0 {
					fault.Merge()
					srp := &tx.SegRemoveParas{
						UserID:   si.userID,
						ProID:    s.localID,
						Nonce:    i,
						SeqNum:   k,
						Segments: fault,
					}

					data, err := srp.Serialize()
					if err != nil {
						return
					}

					msg := &tx.Message{
						Version: 0,
						From:    s.localID,
						To:      si.userID,
						Method:  tx.SegmentFault,
						Params:  data,
					}

					err = s.pushAndWaitMessage(msg)
					if err != nil {
						logger.Debug("challenge push remove seg msg fail: ", err)
					}
					return
				}
			}
		}
	}

	if cnt == 0 {
		logger.Debug("challenge has zero size: ", userID, si.nextChal)
		return
	}

	totalSize -= delSize

	if totalSize == 0 {
		logger.Debug("challenge has zero size: ", userID, si.nextChal)
		return
	}

	// generate proof
	res, err := pf.Result()
	if err != nil {
		logger.Debug("challenge generate proof: ", userID, err)
		return
	}

	ok, err := pk.VerifyKey().VerifyProof(pchal, res)
	if err != nil {
		logger.Debug("challenge generate wrong proof: ", userID, err)
		return
	}

	if !ok {
		logger.Debug("challenge generate wrong proof: ", userID)
		return
	}

	logger.Debugf("challenge create proof: user %d, pro %d, chal %d, nonce %d, seq %d, order start %d, end %d, seg count %d, size %d, price %d, cost %s", userID, s.localID, si.nextChal, ns.Nonce, ns.SeqNum, orderStart, orderEnd, cnt, totalSize, totalPrice, time.Since(nt))

	scp := &tx.SegChalParams{
		UserID:     userID,
		ProID:      s.localID,
		Epoch:      si.nextChal,
		OrderStart: orderStart,
		OrderEnd:   orderEnd,
		Size:       totalSize,
		Price:      totalPrice,
		Proof:      res.Serialize(),
	}

	data, err := scp.Serialize()
	if err != nil {
		logger.Debug("challenge serialize: ", userID, err)
	}

	// submit proof
	msg := &tx.Message{
		Version: 0,
		From:    s.localID,
		To:      si.userID,
		Method:  tx.SegmentProof,
		Params:  data,
	}

	si.chalTime = time.Now()
	si.wait = true
	s.pushMessage(msg)
}
