package challenge

import (
	"context"
	"encoding/binary"
	"sync"
	"time"

	"github.com/bits-and-blooms/bitset"
	"github.com/fxamacker/cbor/v2"
	"github.com/gogo/protobuf/proto"
	"github.com/zeebo/blake3"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/build"
	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12_381"
	"github.com/memoio/go-mefs-v2/lib/crypto/pdp"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/submodule/state"
)

var logger = logging.Logger("pro-challenge")

type SegMgr struct {
	sync.RWMutex
	api.IChain
	api.IState

	ds       store.KVStore
	segStore segment.SegmentStore

	ctx context.Context

	localID uint64

	epoch uint64
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

	pk pdpcommon.PublicKey

	nextChal uint64
	wait     bool
	chalTime time.Time

	bucket  uint64 // total bucket
	buckets []*BucketInfo
}

// each epoch has one
type BucketSet struct {
	Size       uint64
	AvalStripe *bitset.BitSet
	AccHw      bls.Fr
}

type BucketInfo struct {
	BucketSet
	Chunk   uint32
	StQueue types.StripeQueue
}

func NewSegMgr(ctx context.Context, localID uint64, ds store.KVStore, ss segment.SegmentStore, ic api.IChain, is api.IState) *SegMgr {
	s := &SegMgr{
		IChain:   ic,
		IState:   is,
		ctx:      ctx,
		localID:  localID,
		ds:       ds,
		segStore: ss,
		users:    make([]uint64, 0, 128),
		sInfo:    make(map[uint64]*segInfo),

		chalChan: make(chan *chal, 8),
	}

	s.load()

	return s
}

func (s *SegMgr) Start() {
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

func (s *SegMgr) loadFs(userID uint64, save bool) *segInfo {
	si, ok := s.sInfo[userID]
	if !ok {
		key := store.NewKey(pb.MetaType_ST_PDPPublicKey, userID)
		data, err := s.ds.Get(key)
		if err != nil {
			logger.Debug("challenge not get fs")
			return nil
		}

		pk, err := pdp.DeserializePublicKey(data)
		if err != nil {
			logger.Debug("challenge not get fs")
			return nil
		}

		// load from local
		si = &segInfo{
			userID:  userID,
			pk:      pk,
			fsID:    pk.VerifyKey().Hash(),
			buckets: make([]*BucketInfo, 0, 2),
		}

		key = store.NewKey(pb.MetaType_ST_BucketOptKey, userID)
		data, err = s.ds.Get(key)
		if err == nil && len(data) >= 8 {
			si.bucket = binary.BigEndian.Uint64(data)
		}

		si.buckets = make([]*BucketInfo, si.bucket)

		for i := uint64(0); i < si.bucket; i++ {
			bm := &BucketInfo{
				BucketSet: BucketSet{
					AvalStripe: bitset.New(1024),
					AccHw:      bls.ZERO,
				},
			}

			key := store.NewKey(pb.MetaType_Chal_BucketInfoKey, userID, i)
			data, err = s.ds.Get(key)
			if err == nil {
				cbor.Unmarshal(data, bm)
			} else {
				pbo := new(pb.BucketOption)
				key := store.NewKey(pb.MetaType_ST_BucketOptKey, userID, i)
				data, err := s.ds.Get(key)
				if err != nil {
					return si
				}
				err = proto.Unmarshal(data, pbo)
				if err != nil {
					return si
				}
				bm.Chunk = pbo.DataCount + pbo.ParityCount
			}
			si.buckets[i] = bm
		}

		s.users = append(s.users, userID)
		s.sInfo[userID] = si

		if save {
			s.save()
		}
	}

	return si
}

func (s *SegMgr) loadBucket(si *segInfo, bucketID uint64) *BucketInfo {
	if bucketID < si.bucket {
		return si.buckets[bucketID]
	}

	for si.bucket <= bucketID {
		bm := &BucketInfo{
			BucketSet: BucketSet{
				AvalStripe: bitset.New(1024),
				AccHw:      bls.ZERO,
			},
		}

		key := store.NewKey(pb.MetaType_Chal_BucketInfoKey, si.userID, si.bucket)
		data, err := s.ds.Get(key)
		if err == nil {
			cbor.Unmarshal(data, bm)
		} else {
			pbo := new(pb.BucketOption)
			key := store.NewKey(pb.MetaType_ST_BucketOptKey, si.userID, si.bucket)
			data, err := s.ds.Get(key)
			if err != nil {
				return nil
			}
			err = proto.Unmarshal(data, pbo)
			if err != nil {
				return nil
			}
			bm.Chunk = pbo.DataCount + pbo.ParityCount
		}

		si.buckets = append(si.buckets, bm)
		si.bucket++
	}

	return si.buckets[bucketID]
}

func (s *SegMgr) AddStripe(userID, bucketID, stripeStart, stripeLength, proID, epoch uint64, chunkID uint32) {
	logger.Debug("challenge AddStripe for: ", userID, proID)
	s.Lock()
	defer s.Unlock()

	if proID != s.localID {
		return
	}

	si := s.loadFs(userID, true)

	bm := s.loadBucket(si, bucketID)

	stripe := &types.Stripe{
		ChunkID: chunkID,
		Start:   stripeStart,
		Length:  stripeLength,
	}
	bm.StQueue.Push(stripe)

	var HWi bls.Fr
	for i := stripeStart; i < stripeStart+stripeLength; i++ {
		bm.Size += build.DefaultSegSize

		bm.AvalStripe.Set(uint(i))

		// calculate fr
		sid := segment.CreateSegmentID(si.fsID, bucketID, i, chunkID)
		h := blake3.Sum256(sid)
		bls.FrFromBytes(&HWi, h[:])
		bls.FrAddMod(&bm.AccHw, &bm.AccHw, &HWi)
	}

	// save
	key := store.NewKey(pb.MetaType_Chal_BucketInfoKey, userID, bucketID)
	data, err := cbor.Marshal(bm)
	if err != nil {
		logger.Debug("marshal fails:", err)
		return
	}
	s.ds.Put(key, data)

	key = store.NewKey(pb.MetaType_Chal_BucketInfoKey, userID, bucketID, epoch)
	data, err = cbor.Marshal(bm.BucketSet)
	if err != nil {
		logger.Debug("marshal fails:", err)
		return
	}
	s.ds.Put(key, data)
}

func (s *SegMgr) updateEpoch() {
	epoch := s.GetEpoch()
	if epoch <= s.epoch {
		return
	}
	logger.Debug("challenge update epoch: ", epoch)
	s.epoch = epoch
	// save epoch avail map; fr
	for _, userID := range s.users {
		si := s.loadFs(userID, false)
		for i := uint64(0); i < si.bucket; i++ {
			bm := s.loadBucket(si, i)

			// save for chal
			key := store.NewKey(pb.MetaType_Chal_BucketInfoKey, userID, i, epoch-1)
			data, err := cbor.Marshal(bm.BucketSet)
			if err != nil {
				logger.Debug("marshal fails:", err)
				return
			}
			s.ds.Put(key, data)
		}
	}
}

func (s *SegMgr) challenge(userID uint64) {
	logger.Debug("challenge: ", userID)
	si := s.loadFs(userID, false)
	if si.nextChal >= s.epoch {
		logger.Debug("challenged at: ", userID, si.nextChal)
		return
	} else {
		si.nextChal = s.epoch - 1
	}
	if si.wait {
		if time.Since(si.chalTime) > 10*time.Minute {
			si.wait = false
		}
		logger.Debug("challenging at: ", userID, si.nextChal)
		return
	}

	key := store.NewKey(pb.MetaType_ST_SegProof, userID, s.localID, si.nextChal)
	ok, err := s.ds.Has(key)
	if err == nil && ok {
		logger.Debug("challenged: ", userID)
		return
	}

	logger.Debug("challenge: ", userID, si.nextChal)

	// get epoch info
	sce := new(state.ChalEpoch)
	key = store.NewKey(pb.MetaType_ST_EpochKey, si.nextChal)
	val, err := s.ds.Get(key)
	if err != nil {
		logger.Debug("challenge cannot get epoch info: ", si.nextChal)
		return
	}

	err = sce.Deserialize(val)
	if err != nil {
		logger.Debug("challenge cannot des epoch info: ", si.nextChal)
		return
	}

	buf := make([]byte, 8+len(sce.Seed.Bytes()))
	binary.BigEndian.PutUint64(buf[:8], userID)
	copy(buf[8:], sce.Seed.Bytes())
	bh := blake3.Sum256(buf)

	chal, err := pdp.NewChallenge(si.pk.VerifyKey(), bh)
	if err != nil {
		logger.Debug("challenge cannot create chal: ", si.nextChal)
		return
	}

	pf, err := pdp.NewProofAggregator(si.pk, bh)
	if err != nil {
		logger.Debug("challenge cannot create proof: ", si.nextChal)
		return
	}

	sid, err := segment.NewSegmentID(si.fsID, 0, 0, 0)
	if err != nil {
		logger.Debug("chal creaet seg fails:", err)
		return
	}

	size := uint64(0)

	// challenge routine
	for i := uint64(0); i < si.bucket; i++ {
		sid.SetBucketID(i)

		bm := si.buckets[i]
		key := store.NewKey(pb.MetaType_Chal_BucketInfoKey, userID, i, si.nextChal)
		val, err := s.ds.Get(key)
		if err != nil {
			val, err = cbor.Marshal(bm.BucketSet)
			if err != nil {
				logger.Debug("chal marshal fails:", err)
				return
			}
			s.ds.Put(key, val)
		}

		bs := new(BucketSet)
		err = cbor.Unmarshal(val, bs)
		if err != nil {
			logger.Debug("chal unmarshal fails:", err)
			return
		}

		if bs.Size == 0 {
			logger.Debug("chal has zero size: ", i)
			continue
		}

		size += bs.Size

		var HWi, accHw bls.Fr
		for j := uint(0); j < bs.AvalStripe.Len(); j++ {
			if !bs.AvalStripe.Test(j) {
				continue
			}

			cid, err := bm.StQueue.GetChunkID(uint64(j))
			if err != nil {
				logger.Debug("challenge not have chunk for stripe: ", j)
				continue
			}

			sid.SetStripeID(uint64(j))
			sid.SetChunkID(cid)

			h := blake3.Sum256(sid.Bytes())
			bls.FrFromBytes(&HWi, h[:])
			bls.FrAddMod(&accHw, &accHw, &HWi)

			seg, err := s.segStore.Get(sid)
			if err != nil {
				logger.Debug("challenge not have chunk for stripe: ", sid.ShortString())
				continue
			}

			segData, _ := seg.Content()
			segTag, _ := seg.Tags()

			err = pf.Add(sid.Bytes(), segData, segTag[0])
			if err != nil {
				logger.Debug("challenge add to proof: ", sid.ShortString(), err)
				continue
			}
		}
		if !bls.FrEqual(&accHw, &bs.AccHw) {
			logger.Warnf("chal got %s, expect %s", HWi.String(), bs.AccHw.String())
		}

		chal.Add(bls.FrToBytes(&accHw))
	}

	if size == 0 {
		logger.Debug("chal has zero size: ", userID, si.nextChal)
		return
	}

	// generate proof
	res, err := pf.Result()
	if err != nil {
		logger.Debug("challenge generate proof: ", userID, err)
		return
	}

	ok, err = si.pk.VerifyKey().VerifyProof(chal, res)
	if err != nil {
		logger.Debug("challenge generate wrong proof: ", userID, err)
		return
	}

	if !ok {
		logger.Debug("challenge generate wrong proof: ", userID)
		return
	}

	logger.Debug("challenge create proof: ", userID, si.nextChal)

	scp := &tx.SegChalParams{
		Epoch: si.nextChal,
		Proof: res.Serialize(),
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

func (s *SegMgr) regularChallenge() {
	tc := time.NewTicker(time.Minute)
	defer tc.Stop()

	s.updateEpoch()

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
			s.updateEpoch()
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
