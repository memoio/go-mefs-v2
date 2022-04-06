package types

import (
	"sort"

	"github.com/fxamacker/cbor/v2"
	"golang.org/x/xerrors"
)

// sorted by bucketID and jobID
type SegJob struct {
	JobID    uint64
	BucketID uint64
	Start    uint64
	Length   uint64
	ChunkID  uint32
}

func (sj *SegJob) Serialize() ([]byte, error) {
	return cbor.Marshal(sj)
}

func (sj *SegJob) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, sj)
}

type SegJobsQueue []*SegJob

func (sjq SegJobsQueue) Len() int {
	return len(sjq)
}

func (asq SegJobsQueue) Less(i, j int) bool {
	if asq[i].BucketID != asq[j].BucketID {
		return asq[i].BucketID < asq[j].BucketID
	}

	if asq[i].JobID != asq[j].JobID {
		return asq[i].JobID < asq[j].JobID
	}

	return asq[i].Start < asq[j].Start
}

func (sjq SegJobsQueue) Swap(i, j int) {
	sjq[i], sjq[j] = sjq[j], sjq[i]
}

//  after sort
func (sjq SegJobsQueue) Has(bucketID, jobID, stripeID uint64, chunkID uint32) bool {
	for _, as := range sjq {
		if as.BucketID < bucketID {
			continue
		}

		if as.BucketID > bucketID {
			break
		}

		if as.JobID < jobID {
			continue
		}

		if as.JobID > jobID {
			break
		}

		if as.Start > stripeID {
			break
		} else {
			if as.Start <= stripeID && as.Start+as.Length > stripeID && as.ChunkID == chunkID {
				return true
			}
		}

	}

	return false
}

func (sjq SegJobsQueue) Equal(old SegJobsQueue) bool {
	if sjq.Len() != old.Len() {
		return false
	}

	for i := 0; i < sjq.Len(); i++ {
		if sjq[i].BucketID != old[i].BucketID || sjq[i].JobID != old[i].JobID || sjq[i].Start != old[i].Start || sjq[i].Length != old[i].Length {
			return false
		}
	}

	return true
}

func (sjq *SegJobsQueue) Push(s *SegJob) {
	if sjq.Len() == 0 {
		*sjq = append(*sjq, s)
		return
	}

	asqval := *sjq
	last := asqval[sjq.Len()-1]
	if last.BucketID == s.BucketID && last.JobID == s.JobID && last.Start+last.Length == s.Start {
		last.Length += s.Length
		*sjq = asqval
	} else {
		*sjq = append(*sjq, s)
	}
}

func (asq *SegJobsQueue) Merge() {
	sort.Sort(asq)
	aLen := asq.Len()
	asqval := *asq
	for i := 0; i < aLen-1; {
		j := i + 1
		for ; j < aLen; j++ {
			if asqval[i].BucketID == asqval[j].BucketID && asqval[i].JobID == asqval[j].JobID && asqval[i].Start+asqval[i].Length == asqval[j].Start {
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

func (sjq *SegJobsQueue) Serialize() ([]byte, error) {
	return cbor.Marshal(sjq)
}

func (sjq *SegJobsQueue) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, sjq)
}

// aggreated segs in order
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

func (asq AggSegsQueue) Size() int {
	res := 0
	for _, as := range asq {
		res += int(as.Length)
	}
	return res
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
func (asq AggSegsQueue) Has(bucketID, stripeID uint64, chunkID uint32) bool {
	for _, as := range asq {
		if as.BucketID > bucketID {
			break
		} else if as.BucketID == bucketID {
			if as.Start > stripeID {
				break
			} else {
				if as.Start <= stripeID && as.Start+as.Length > stripeID && as.ChunkID == chunkID {
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

// stripe in state
type AggStripe struct {
	Order  uint64
	ProID  uint64
	Start  uint64
	Length uint64
}

func (as *AggStripe) Serialize() ([]byte, error) {
	return cbor.Marshal(as)
}

func (as *AggStripe) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, as)
}

// stripe in provider
type Stripe struct {
	ChunkID uint32
	Start   uint64
	Length  uint64
}

type StripeQueue []*Stripe

func (sq StripeQueue) Len() int {
	return len(sq)
}

func (sq StripeQueue) Less(i, j int) bool {
	return sq[i].Start < sq[j].Start
}

func (sq StripeQueue) Swap(i, j int) {
	sq[i], sq[j] = sq[j], sq[i]
}

//  after sort
func (sq StripeQueue) Has(stripeID uint64, chunkID uint32) bool {
	for _, as := range sq {
		if as.Start > stripeID {
			break
		} else {
			if as.Start <= stripeID && as.Start+as.Length > stripeID && as.ChunkID == chunkID {
				return true
			}
		}
	}

	return false
}

func (sq StripeQueue) GetChunkID(stripeID uint64) (uint32, error) {
	for _, as := range sq {
		if as.Start > stripeID {
			break
		} else {
			if as.Start <= stripeID && as.Start+as.Length > stripeID {
				return as.ChunkID, nil
			}
		}
	}

	return 0, xerrors.Errorf("%d not found", stripeID)
}

func (sq StripeQueue) Equal(old StripeQueue) bool {
	if sq.Len() != old.Len() {
		return false
	}

	for i := 0; i < sq.Len(); i++ {
		if sq[i].ChunkID != old[i].ChunkID || sq[i].Start != old[i].Start || sq[i].Length != old[i].Length {
			return false
		}
	}

	return true
}

func (sq *StripeQueue) Push(s *Stripe) {
	if sq.Len() == 0 {
		*sq = append(*sq, s)
		return
	}

	asqval := *sq
	last := asqval[sq.Len()-1]
	if last.ChunkID == s.ChunkID && last.Start+last.Length == s.Start {
		last.Length += s.Length
		*sq = asqval
	} else {
		*sq = append(*sq, s)
	}
}

func (sq *StripeQueue) Merge() {
	sort.Sort(sq)
	aLen := sq.Len()
	asqval := *sq
	for i := 0; i < aLen-1; {
		j := i + 1
		for ; j < aLen; j++ {
			if asqval[i].ChunkID == asqval[j].ChunkID && asqval[i].Start+asqval[i].Length == asqval[j].Start {
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

	*sq = asqval
}

func (sq *StripeQueue) Serialize() ([]byte, error) {
	return cbor.Marshal(sq)
}

func (sq *StripeQueue) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, sq)
}
