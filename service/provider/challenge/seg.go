package challenge

import (
	"context"
	"encoding/binary"
	"math/big"
	"sync"
	"time"

	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/crypto/pdp"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/submodule/txPool"
	"github.com/zeebo/blake3"
)

var logger = logging.Logger("pro-challenge")

type SegMgr struct {
	sync.RWMutex

	*txPool.PushPool

	ds       store.KVStore
	segStore segment.SegmentStore

	ctx context.Context

	localID uint64

	epoch uint64
	eInfo *types.ChalEpoch

	users []uint64
	sInfo map[uint64]*segInfo // key: userID

	chalChan chan *chal
}

type chal struct {
	userID  uint64
	epoch   uint64
	errCode uint16
}

type segInfo struct {
	userID uint64
	fsID   []byte
	pk     pdpcommon.PublicKey

	nextChal uint64
	wait     bool
	chalTime time.Time
}

func NewSegMgr(ctx context.Context, localID uint64, ds store.KVStore, ss segment.SegmentStore, pp *txPool.PushPool) *SegMgr {
	s := &SegMgr{
		PushPool: pp,
		ctx:      ctx,
		localID:  localID,
		ds:       ds,
		segStore: ss,
		users:    make([]uint64, 0, 128),
		sInfo:    make(map[uint64]*segInfo),

		chalChan: make(chan *chal, 8),
	}

	return s
}

func (s *SegMgr) Start() {
	s.load()
	go s.regularChallenge()
}

func (s *SegMgr) load() {
	key := store.NewKey(pb.MetaType_Chal_UsersKey)
	val, err := s.ds.Get(key)
	if err != nil {
		return
	}

	for i := 0; i < len(val)/8; i++ {
		pid := binary.BigEndian.Uint64(val[8*i : 8*(i+1)])
		s.loadFs(pid, false)
	}
}

func (s *SegMgr) save() error {
	buf := make([]byte, 8*len(s.users))
	for i, pid := range s.users {
		binary.BigEndian.PutUint64(buf[8*i:8*(i+1)], pid)
	}

	key := store.NewKey(pb.MetaType_Chal_UsersKey)
	return s.ds.Put(key, buf)
}

func (s *SegMgr) AddUser(userID uint64) {
	go s.loadFs(userID, true)
}

func (s *SegMgr) loadFs(userID uint64, save bool) *segInfo {
	si, ok := s.sInfo[userID]
	if !ok {
		pk, err := s.GetPDPPublicKey(s.ctx, userID)
		if err != nil {
			logger.Debug("challenge not get fs")
			return nil
		}

		// load from local
		si = &segInfo{
			userID: userID,
			pk:     pk,
			fsID:   pk.VerifyKey().Hash(),
		}

		s.users = append(s.users, userID)
		s.sInfo[userID] = si

		if save {
			s.save()
		}
	}

	return si
}

func (s *SegMgr) regularChallenge() {
	tc := time.NewTicker(time.Minute)
	defer tc.Stop()

	s.epoch = s.GetChalEpoch(s.ctx)
	s.eInfo = s.GetChalEpochInfo(s.ctx)

	i := 0
	for {
		select {
		case <-s.ctx.Done():
			return
		case ch := <-s.chalChan:
			si := s.loadFs(ch.userID, true)
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
			s.eInfo = s.GetChalEpochInfo(s.ctx)
			logger.Debug("challenge update epoch: ", s.eInfo.Epoch, s.epoch)
		default:
			if len(s.users) == 0 {
				logger.Debug("challenge no users")
				time.Sleep(10 * time.Second)
				continue
			}

			if len(s.users) >= i {
				i = 0
				time.Sleep(10 * time.Second)
			}

			userID := s.users[i]
			s.challenge(userID)
			i++
		}
	}
}

func (s *SegMgr) challenge(userID uint64) {
	logger.Debug("challenge: ", userID)
	if s.epoch < 2 {
		logger.Debug("not challenge at epoch: ", userID, s.epoch)
		return
	}
	si := s.loadFs(userID, false)
	if si.nextChal > s.eInfo.Epoch {
		logger.Debug("challenged at: ", userID, s.eInfo.Epoch)
		return
	} else {
		si.nextChal = s.eInfo.Epoch
	}

	if si.wait {
		if time.Since(si.chalTime) > 10*time.Minute {
			si.wait = false
			logger.Debug("redo challenge at: ", userID, si.nextChal)
		}
		return
	}

	if s.GetProof(userID, s.localID, si.nextChal) {
		logger.Debug("has challenged: ", userID, si.nextChal)
		return
	}

	// get epoch info

	buf := make([]byte, 8+len(s.eInfo.Seed.Bytes()))
	binary.BigEndian.PutUint64(buf[:8], userID)
	copy(buf[8:], s.eInfo.Seed.Bytes())
	bh := blake3.Sum256(buf)

	chal, err := pdp.NewChallenge(si.pk.VerifyKey(), bh)
	if err != nil {
		logger.Debug("challenge cannot create chal: ", userID, si.nextChal)
		return
	}

	pf, err := pdp.NewProofAggregator(si.pk, bh)
	if err != nil {
		logger.Debug("challenge cannot create proof: ", userID, si.nextChal)
		return
	}

	sid, err := segment.NewSegmentID(si.fsID, 0, 0, 0)
	if err != nil {
		logger.Debug("chal create seg fails:", userID, si.nextChal, err)
		return
	}

	cnt := uint64(0)

	// challenge routine
	ns := s.GetOrderStateAt(userID, s.localID, si.nextChal)
	if ns.Nonce == 0 && ns.SeqNum == 0 {
		logger.Debug("chal on empty data at epoch: ", userID, si.nextChal)
		return
	}

	pce, err := s.GetChalEpochInfoAt(s.ctx, si.nextChal-1)
	if err != nil {
		logger.Debug("chal at wrong epoch: ", userID, si.nextChal, err)
		return
	}

	chalStart := build.BaseTime + int64(pce.Slot*build.SlotDuration)
	chalEnd := build.BaseTime + int64(s.eInfo.Slot*build.SlotDuration)
	chalDur := chalEnd - chalStart
	if chalDur <= 0 {
		logger.Debug("chal at wrong epoch: ", userID, si.nextChal)
		return
	}

	orderDur := int64(0)
	price := new(big.Int)
	totalPrice := new(big.Int)
	totalSize := uint64(0)

	orderStart := uint64(0)
	orderEnd := uint64(0)

	if ns.Nonce > 0 {
		so, _, _, err := s.GetOrder(userID, s.localID, ns.Nonce-1)
		if err != nil {
			logger.Debug("chal get order fails: ", userID, ns.Nonce-1, err)
			return
		}

		if so.Start >= chalEnd || so.End <= chalStart {
			logger.Debug("chal order all expired: ", userID, si.nextChal)
			return
		}
		orderEnd = ns.Nonce - 1

		for i := uint32(0); i < ns.SeqNum; i++ {
			seq, accFr, err := s.GetOrderSeq(userID, s.localID, ns.Nonce-1, i)
			if err != nil {
				logger.Debug("chal get order seq fails: ", userID, ns.Nonce-1, i, err)
				return
			}

			if i == ns.SeqNum-1 {
				if so.Start <= chalStart && so.End >= chalEnd {
					orderDur = chalDur
				} else if so.Start <= chalStart && so.End <= chalEnd {
					orderDur = so.End - chalStart
				} else if so.Start >= chalStart && so.End >= chalEnd {
					orderDur = chalEnd - so.Start
				} else if so.Start >= chalStart && so.End <= chalEnd {
					orderDur = so.End - so.Start
				}
				price.Set(seq.Price)
				price.Mul(price, big.NewInt(orderDur))
				totalPrice.Add(totalPrice, price)
				totalSize += seq.Size
				chal.Add(accFr)
			}

			for _, seg := range seq.Segments {
				sid.SetBucketID(seg.BucketID)
				for j := seg.Start; j < seg.Start+seg.Length; j++ {
					sid.SetStripeID(j)
					sid.SetChunkID(seg.ChunkID)
					segm, err := s.segStore.Get(sid)
					if err != nil {
						logger.Debug("challenge not have chunk for stripe: ", userID, sid.ShortString())
						continue
					}

					segData, _ := segm.Content()
					segTag, _ := segm.Tags()

					err = pf.Add(sid.Bytes(), segData, segTag[0])
					if err != nil {
						logger.Debug("challenge add to proof: ", userID, sid.ShortString(), err)
						continue
					}
					cnt++
				}
			}

		}
	}

	if ns.Nonce > 1 {
		// todo: choose some from [0, ns.Nonce-1)
		for i := uint64(0); i < ns.Nonce-1; i++ {
			so, accFr, seqNum, err := s.GetOrder(userID, s.localID, i)
			if err != nil {
				logger.Debug("chal get order fails:", userID, i, err)
				return
			}

			if so.Start >= chalEnd || so.End <= chalStart {
				logger.Debug("chal order expired: ", userID, si.nextChal, i)
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
			price.Mul(price, big.NewInt(orderDur))
			totalPrice.Add(totalPrice, price)
			totalSize += so.Size

			chal.Add(accFr)

			for k := uint32(0); k < seqNum; k++ {
				seq, _, err := s.GetOrderSeq(userID, s.localID, i, k)
				if err != nil {
					logger.Debug("chal get order seq fails:", userID, i, k, err)
					return
				}
				for _, seg := range seq.Segments {
					sid.SetBucketID(seg.BucketID)
					for j := seg.Start; j < seg.Start+seg.Length; j++ {
						sid.SetStripeID(j)
						sid.SetChunkID(seg.ChunkID)

						segm, err := s.segStore.Get(sid)
						if err != nil {
							logger.Debug("challenge not have chunk for stripe: ", userID, sid.ShortString())
							continue
						}

						segData, _ := segm.Content()
						segTag, _ := segm.Tags()

						err = pf.Add(sid.Bytes(), segData, segTag[0])
						if err != nil {
							logger.Debug("challenge add to proof: ", userID, sid.ShortString(), err)
							continue
						}
						cnt++
					}
				}
			}
		}
	}

	if cnt == 0 {
		logger.Debug("chal has zero size: ", userID, si.nextChal)
		return
	}

	if totalSize == 0 {
		logger.Debug("chal has zero size: ", userID, si.nextChal)
		return
	}

	// generate proof
	res, err := pf.Result()
	if err != nil {
		logger.Debug("challenge generate proof: ", userID, err)
		return
	}

	ok, err := si.pk.VerifyKey().VerifyProof(chal, res)
	if err != nil {
		logger.Debug("challenge generate wrong proof: ", userID, err)
		return
	}

	if !ok {
		logger.Debug("challenge generate wrong proof: ", userID)
		return
	}

	logger.Debug("challenge create proof: ", userID, s.localID, si.nextChal, ns.Nonce, ns.SeqNum, orderStart, orderEnd, cnt, totalSize, totalPrice)

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
	s.pushMessage(msg, si.nextChal)
}
