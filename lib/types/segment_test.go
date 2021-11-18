package types

import (
	"crypto/md5"
	"encoding/hex"
	"math/big"
	"sort"
	"testing"

	"github.com/zeebo/blake3"
)

func TestAggSegs(t *testing.T) {
	var asq AggSegsQueue

	as1 := &AggSegs{
		BucketID: 1,
		Start:    5,
		Length:   3,
	}

	as2 := &AggSegs{
		BucketID: 1,
		Start:    0,
		Length:   5,
	}

	as3 := &AggSegs{
		BucketID: 1,
		Start:    8,
		Length:   7,
	}

	as4 := &AggSegs{
		BucketID: 0,
		Start:    8,
		Length:   7,
	}

	as5 := &AggSegs{
		BucketID: 0,
		Start:    15,
		Length:   10,
	}

	id := blake3.Sum256([]byte("test"))

	asq.Push(as1)
	asq.Push(as2)
	asq.Push(as3)
	asq.Push(as4)
	asq.Push(as5)

	t.Log(asq.Len(), asq[3])

	sort.Sort(asq)

	asq.Merge()

	t.Log(asq.Len())

	os := new(OrderSeq)
	os.Segments = asq
	os.Size = 1
	os.Price = big.NewInt(10)
	os.ID = id

	os.Segments.Push(as1)

	osbyte, _ := os.Serialize()

	md5val := md5.Sum(osbyte)
	t.Log(hex.EncodeToString(md5val[:]))

	nos := new(OrderSeq)
	nos.Deserialize(osbyte)

	nos.Price = big.NewInt(10)
	nos.Size = 1
	nos.ID = id

	nosbyte, _ := nos.Serialize()

	md5val = md5.Sum(nosbyte)
	t.Log(hex.EncodeToString(md5val[:]))

	t.Fatal(asq[0].Length, asq.Len(), asq)
}
