package types

import (
	"sort"
)

// sorted by bucketID and jobID
type SegJob struct {
	JobID    uint64
	BucketID uint64
	Start    uint64
	Length   uint64
	ChunkID  uint32
}

// aggreated segs
type AggSegs struct {
	BucketID uint64
	Start    uint64
	Length   uint64
	ChunkID  uint32
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

//  after sort
func (asq AggSegsQueue) Has(bucketID, stripeID uint64) bool {
	for _, as := range asq {
		if as.BucketID > bucketID {
			break
		} else if as.BucketID == bucketID {
			if as.Start > stripeID {
				break
			} else {
				if as.Start <= stripeID && as.Start+as.Length > stripeID {
					return true
				}
			}
		}
	}

	return false
}

func (asq AggSegsQueue) Equal(old AggSegsQueue) bool {
	if asq.Len() != old.Len() {
		return false
	}

	for i := 0; i < asq.Len(); i++ {
		if asq[i].BucketID != old[i].BucketID || asq[i].Start != old[i].Start || asq[i].Length != old[i].Length {
			return false
		}
	}

	return true
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

type Stripe struct {
	Start uint64 // stripe start
	Val   uint64 // proID/expire; proID is Max.Uint64 when faulted
}

// Stripe
type AggStripe struct {
	Nonce  uint64
	ProID  uint64
	Start  uint64
	Length uint64
}

type AggStripeQueue []*AggStripe

func (sq AggStripeQueue) Len() int {
	return len(sq)
}

func (sq *AggStripeQueue) Push(s *AggStripe) {
	if sq.Len() == 0 {
		*sq = append(*sq, s)
	}

	sqval := *sq
	last := sqval[sq.Len()-1]
	if last.ProID == s.ProID && last.Start+last.Length == s.Start {
		last.Length += s.Length
		*sq = sqval
	} else {
		*sq = append(*sq, s)
	}
}

func (sq AggStripeQueue) Has(stripeID uint64) bool {
	for _, as := range sq {
		if as.Start > stripeID {
			break
		} else {
			if as.Start <= stripeID && as.Start+as.Length > stripeID {
				return true
			}
		}
	}

	return false
}
