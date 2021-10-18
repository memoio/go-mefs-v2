package types

import (
	"fmt"
	"math/rand"
	"strconv"
	"testing"
	"time"

	mt "github.com/iden3/go-iden3-core/merkletree"
)

func BenchmarkSMT(b *testing.B) {
	path := "/home/fjt/smt-test"
	smt, err := NewStateTree(path)
	if err != nil {
		panic(err)
	}
	defer smt.Close()

	prevRoot := smt.GetRoot()

	_, err = smt.DB().Get(prevRoot)
	if err != nil {
		panic(err)
	}

	rand.Seed(time.Now().UnixNano())
	smt.RunGC()
	fmt.Println(time.Now())
	for i := 0; i < 500000; i++ {
		t := rand.Int()
		//log.Println(t)
		key := []byte(strconv.Itoa(i))
		val := []byte(strconv.Itoa(t))

		err := smt.Put(key, val)
		if err != nil {
			panic(err)
		}
		//log.Println(smt.DB().GetCount())
	}

	fmt.Println(time.Now())
	for i := 0; i < 50; i++ {
		smt.RunGC()
	}

	return

	for {
		fmt.Println(prevRoot)
		val, err := smt.DB().Get(concat([]byte(PrevRootPrefix), prevRoot))
		if err != nil {

			panic(err)
		}
		fmt.Println("rootlen: ", len(smt.GetRoot()))
		fmt.Println("delete: ", val)

		value, err := smt.Get(val[32:])
		if err != nil {
			panic(err)
		}

		fmt.Println("value: ", value)

		res, err := smt.Delete(val[32:], val[:32])
		if err != nil {
			panic(err)
		}
		fmt.Println("res: ", res)
		fmt.Println("delete for key: ", val[32:])
		prevRoot = val[:32]
	}
}

func BenchmarkMT(b *testing.B) {
	path := "/home/fjt/smt-test"
	smt, err := NewMTree(path)
	if err != nil {
		panic(err)
	}
	defer smt.Storage().Close()
	rand.Seed(time.Now().Unix())
	fmt.Println(time.Now())
	for i := 0; i < 100000; i++ {
		//key := []byte(strconv.Itoa(rand.Int()))
		e := mt.NewEntryFromInts(int64(i), int64(i), int64(i), int64(i), int64(i), int64(i), int64(i), int64(i))

		smt.AddEntry(&e)
	}

	fmt.Println(time.Now())
	return

}
