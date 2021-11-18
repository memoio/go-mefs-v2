package types

import (
	"fmt"
	"sort"
	"sync"

	"github.com/bits-and-blooms/bitset"
	mpb "github.com/memoio/go-mefs-v2/lib/pb"
)

var FaultID uint64 = 1<<64 - 1

// aggreated segs
type AggSegs struct {
	BucketID uint64
	Start    uint64
	Length   uint64
}

type AggSegsQueue []*AggSegs

func (asq AggSegsQueue) Len() int {
	return len(asq)
}

func (asq AggSegsQueue) Less(i, j int) bool {
	if asq[i].BucketID != asq[j].BucketID {
		return asq[i].BucketID < asq[j].BucketID
	}

	return asq[i].Start < asq[j].Start
}

func (asq AggSegsQueue) Swap(i, j int) {
	asq[i], asq[j] = asq[j], asq[i]
}

func (asq *AggSegsQueue) Push(s *AggSegs) {
	if asq.Len() == 0 {
		*asq = append(*asq, s)
		return
	}

	asqval := *asq
	last := asqval[asq.Len()-1]
	if last.BucketID == s.BucketID && last.Start+last.Length == s.Start {

		last.Length += s.Length
		*asq = asqval
		fmt.Println(last.Length, s.Length)
	} else {
		*asq = append(*asq, s)
	}

}

func (asq *AggSegsQueue) Merge() {
	sort.Sort(asq)
	aLen := asq.Len()
	asqval := *asq
	for i := 0; i < aLen-1; {
		j := i + 1
		for ; j < aLen; j++ {
			if asqval[i].BucketID == asqval[j].BucketID && asqval[i].Start+asqval[i].Length == asqval[j].Start {
				asqval[i].Length += asqval[j].Length
				asqval[j].Length = 0
			} else {
				break
			}
		}
		i = j
	}

	j := 0
	for i := 0; i < aLen; i++ {
		if asqval[i].Length == 0 {
			continue
		}

		asqval[j] = asqval[i]
		j++
	}

	asqval = asqval[:j]

	*asq = asqval
}

// sorted by bucketID and jobID
type SegJob struct {
	JobID    uint64
	BucketID uint64
	Start    uint64
	Length   uint64
	ChunkID  uint32
}

// Stripe
type Stripe struct {
	start uint64 // stripe start
	val   uint64 // proID/expire; proID is Max.Uint64 when faulted
}

// B = 12+8*stripeNum/xx
// ChunkManage manage each chunk
type ChunkManage struct {
	chunkID   uint32   // chunkID
	stripeNum uint64   // largest stripe number in this chunk
	stripes   []Stripe // 递增
}

// 二分查找;找到，返回stripe，没有返回error
func (c ChunkManage) GetStripeStart(stripeID uint64) (Stripe, error) {
	return Stripe{}, nil
}

// A=24+16+100*B
// BucketManage manage each bucket
type BucketManage struct {
	sync.RWMutex
	mpb.BucketOption                    // bucket options
	bucketID         uint64             // bucketID
	stripeNum        uint64             // largest stripe number
	stripeExp        uint64             // smallest stripe number in next expire
	expire           uint64             // largest expire time
	expMap           map[uint64]*Stripe // for verify expire; key is expire
	segs             []*ChunkManage     // each chunkID
	accHw            map[uint64][]byte  // key: proID; // 聚合后的hashToFr
	blockSet         *bitset.BitSet     // block stripes; for banned?
}

// segment meta manage
type SegManage struct {
	sync.RWMutex
	bucketMax uint64 // load from chain
	bucketNum uint64 // next bucket number

	buckets []*BucketManage
}

func (si *SegManage) CheckBucket(bucketID uint64) error {
	if bucketID >= si.bucketMax {
		return ErrKeyExists
	}

	if bucketID != si.bucketNum {
		return ErrKeyExists
	}

	return nil
}

func (si *SegManage) AddBucket(bucketID uint64, opt mpb.BucketOption) error {
	si.Lock()
	defer si.Unlock()

	err := si.CheckBucket(bucketID)
	if err != nil {
		return err
	}

	cm := make([]*ChunkManage, opt.DataCount+opt.ParityCount)
	for i := 0; i < int(opt.DataCount+opt.ParityCount); i++ {
		cm[i].chunkID = uint32(i)
	}

	bm := &BucketManage{
		BucketOption: opt,
		bucketID:     bucketID,
		segs:         cm,
	}
	si.buckets = append(si.buckets, bm)

	return nil
}

func (si *SegManage) GetBucket(bucketID uint64) (*BucketManage, error) {
	if bucketID > si.bucketNum {
		return nil, ErrKeyExists
	}

	if int(bucketID) >= len(si.buckets) {
		return nil, ErrKeyExists
	}

	return si.buckets[bucketID], nil
}

func (si *SegManage) CheckSeg(bucketID, stripeID, proID, length, expire uint64, chunkID uint32) error {
	bm, err := si.GetBucket(bucketID)
	if err != nil {
		return err
	}

	// verify chunkID
	if int(chunkID) >= len(bm.segs) || int(chunkID) < 0 {
		return ErrKeyExists
	}

	// stripe exist?
	if stripeID != bm.stripeNum {
		return ErrKeyExists
	}

	// verify stripe expire
	if expire <= bm.expire {
		eStripe, ok := bm.expMap[expire]
		if !ok {
			return ErrKeyExists
		}

		if stripeID < eStripe.start {
			return ErrKeyExists
		}

		if stripeID > eStripe.start+eStripe.val {
			return ErrKeyExists
		}
	} else {
		// should not exist
		_, ok := bm.expMap[expire]
		if ok {
			return ErrKeyExists
		}

		eStripe, ok := bm.expMap[bm.expire]
		if !ok {
			return ErrKeyExists
		}

		if stripeID != eStripe.start+eStripe.val {
			return ErrKeyExists
		}
	}

	// verify stripe chunk
	cm := bm.segs[chunkID]
	if cm.stripeNum != stripeID {
		return ErrKeyExists
	}

	return nil
}

func (si *SegManage) AddSeg(bucketID, stripeID, proID, length, expire uint64, chunkID uint32) error {
	bm, err := si.GetBucket(bucketID)
	if err != nil {
		return err
	}

	// add stripe expire
	if expire <= bm.expire {
		eStripe, ok := bm.expMap[expire]
		if !ok {
			return ErrKeyExists
		}

		if stripeID == eStripe.start+eStripe.val {
			eStripe.val += length
		} else if stripeID < eStripe.start || stripeID > eStripe.start+eStripe.val {
			return ErrKeyExists
		}
	} else {
		s := &Stripe{
			start: stripeID,
			val:   length,
		}

		bm.expMap[expire] = s
		bm.expire = expire
	}

	// add stripe chunk
	cm := bm.segs[chunkID]
	slen := len(cm.stripes)
	if cm.stripes[slen-1].val != proID {
		s := Stripe{
			start: stripeID,
			val:   proID,
		}

		cm.stripes = append(cm.stripes, s)
	}

	cm.stripeNum += length

	// add to stateDB

	return nil
}

func (si *SegManage) GetSeg() error {
	return nil
}

// for verify and challenge
type SegInfo struct {
	fsID      uint64
	userID    uint64 // belongs to which user
	groupID   uint64 // belongs to which keeper group
	level     uint32 // security level
	keepers   []uint64
	providers []uint64

	seg *SegManage

	applied       uint64 // applied height
	root          []byte // merkel root
	lastChallenge uint64
}

type SegMgr struct {
	sync.RWMutex
	size  uint64              // total
	sInfo map[uint64]*SegInfo // key: fsID
}

type faultSegment struct {
	fsID       uint64
	orderNonce uint64   // 在哪个order内，用于计算size, lost
	segs       []string // bucketID_chunkID_stripeID/length
}
