package types

import (
	"crypto/md5"
	"encoding/hex"
	"math/big"
	"sort"
	"testing"
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

	asq.Push(as1)
	asq.Push(as4)
	asq.Push(as2)
	asq.Push(as5)
	asq.Push(as3)

	t.Log(asq.Len(), asq[3])

	sort.Sort(asq)

	t.Log("has:", asq.Has(0, 0, 0), asq.Has(0, 8, 0), asq.Has(0, 13, 0), asq.Has(0, 25, 0))

	asq.Merge()

	t.Log(asq.Len())

	os := new(SignedOrderSeq)
	os.Segments = asq
	os.Size = 1
	os.Price = big.NewInt(10)

	os.Segments.Push(as1)

	osbyte, _ := os.Serialize()

	md5val := md5.Sum(osbyte)
	t.Log(hex.EncodeToString(md5val[:]))

	nos := new(SignedOrderSeq)
	nos.Deserialize(osbyte)

	nos.Price = big.NewInt(10)
	nos.Size = 1

	nosbyte, _ := nos.Serialize()

	md5val = md5.Sum(nosbyte)
	t.Log(hex.EncodeToString(md5val[:]))

	t.Fatal(asq[0].Length, asq.Len(), asq)
}
