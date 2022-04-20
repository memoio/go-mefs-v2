package smt

import (
	"bytes"
	"io/ioutil"
	"math/rand"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/memoio/go-mefs-v2/lib/backend/kv"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

var (
	testKey = []byte("/test")
	testVal = []byte("aaaaa")
)

func initSMTree(t *testing.T) (store.SMTStore, string) {
	path1, err := ioutil.TempDir(os.TempDir(), "testing_smt_")
	if err != nil {
		t.Fatal(err)
	}

	db, err := kv.NewBadgerStore(path1, nil)
	if err != nil {
		t.Fatal(err)
	}

	trie := NewSMTree(nil, db, db)
	return trie, path1
}

func TestSMTBasic(t *testing.T) {
	smtree, path1 := initSMTree(t)

	defer func() {
		smtree.Close()
		os.RemoveAll(path1)
	}()

	err := smtree.NewTxn()
	if err != nil {
		t.Fatal(err)
	}

	// put and commit
	oldRoot := smtree.Root()
	err = smtree.Put(testKey, testVal)
	if err != nil {
		t.Fatal(err)
	}
	val, err := smtree.Get(testKey)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(val, testVal) {
		t.Fatalf("put value: %s, get value %s\n", testVal, val)
	}
	if !bytes.Equal(oldRoot, smtree.Root()) {
		t.Fatal("Root has been modifed before committing")
	}
	smtree.Commit()
	smtree.Discard()

	// verify the root
	curRoot := smtree.Root()
	if bytes.Equal(curRoot, oldRoot) {
		t.Fatal("Root has not been modified after committing")
	}
	// get
	getVal, err := smtree.Get(testKey)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(getVal, testVal) {
		t.Fatalf("get value: %s, expect %s\n", getVal, testVal)
	}

	// test intermediate clean
	// TODO: maybe should use the number of keys to replace the size
	err = smtree.NewTxn()
	if err != nil {
		t.Fatal(err)
	}
	testVal2 := []byte("bbbbb")
	err = smtree.Put(testKey, testVal2)
	if err != nil {
		t.Fatal(err)
	}
	testVal3 := []byte("ccccc")
	err = smtree.Put(testKey, testVal3)
	if err != nil {
		t.Fatal(err)
	}
	smtree.Commit()
	smtree.Discard()
	curRoot = smtree.Root()

	// test no commit
	err = smtree.NewTxn()
	if err != nil {
		t.Fatal(err)
	}
	err = smtree.Put(testKey, testVal2)
	if err != nil {
		t.Fatal(err)
	}
	smtree.Discard()
	if !bytes.Equal(curRoot, smtree.Root()) {
		t.Fatal("Root has been modifed after discarding")
	}
	curVal, _ := smtree.Get(testKey)
	if !bytes.Equal(curVal, testVal3) {
		t.Fatalf("get value: %s, expect %s\n", curVal, testVal3)
	}
}

func TestSMTRewind(t *testing.T) {
	smtree, path1 := initSMTree(t)
	var keys [][]byte
	var roots [][]byte

	defer func() {
		smtree.Close()
		os.RemoveAll(path1)
	}()

	// emulates block executions
	for i := 0; i < 5; i++ {
		err := smtree.NewTxn()
		if err != nil {
			t.Fatal(err)
		}
		tKey := append(testKey, []byte(strconv.Itoa(i))...)
		err = smtree.Put(tKey, testVal)
		if err != nil {
			t.Fatal(err)
		}
		smtree.Commit()
		smtree.Discard()
		keys = append(keys, tKey)
		roots = append(roots, smtree.Root())
	}

	// rollback
	for i := 1; i <= 4; i++ {
		tempkeys := [][]byte{keys[5-i]}
		err := smtree.Rewind(roots[len(roots)-i-1], roots[len(roots)-i], tempkeys)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(smtree.Root(), roots[len(roots)-i-1]) {
			t.Fatal("Root has not been rewind")
		}
		if ok, _ := smtree.Has(keys[5-i]); ok {
			t.Fatal("Key still exists, rewind wrong")
		}
		if ok, _ := smtree.Has(keys[5-i-1]); !ok {
			t.Fatal("Key not exists, rewind wrong")
		}
	}

	tVal, err := smtree.Get(keys[0])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(testVal, tVal) {
		t.Fatalf("Value is not equal, get %s, expect %s\n", tVal, testVal)
	}
}

// Test all block-ops in bulk.
func TestSparseMerkleTree(t *testing.T) {
	for i := 0; i < 5; i++ {
		// Test more inserts/updates than deletions.
		bulkOperations(t, 200, 100, 100, 50)
	}
	for i := 0; i < 5; i++ {
		// 	// Test extreme deletions.
		bulkOperations(t, 200, 100, 100, 500)
	}
}

// Test all block-ops in bulk, with specified ratio probabilities of insert, update and delete.
func bulkOperations(t *testing.T, blocks int, insert int, update int, del int) {
	smt, path1 := initSMTree(t)

	defer func() {
		smt.Close()
		os.RemoveAll(path1)
	}()

	max := insert + update + del
	// 记录每个版本的kv
	kv := make([]map[string]string, blocks)
	for i := 0; i < blocks; i++ {
		kv[i] = make(map[string]string)
	}
	roots := make([][]byte, blocks)
	rand.Seed(time.Now().UnixNano())

	for i := 0; i < blocks; i++ {
		if i != 0 {
			for k, v := range kv[i-1] {
				kv[i][k] = v
			}
		}
		err := smt.NewTxn()
		if err != nil {
			t.Fatal(err)
		}
		for j := 0; j < 50; j++ {
			n := rand.Intn(max)
			if n < insert { // Insert
				keyLen := 16 + rand.Intn(32)
				key := make([]byte, keyLen)
				rand.Read(key)

				valLen := 1 + rand.Intn(64)
				val := make([]byte, valLen)
				rand.Read(val)

				kv[i][string(key)] = string(val)
				err := smt.Put(key, val)
				if err != nil {
					t.Errorf("error: %v", err)
				}
			} else if n > insert && n < insert+update { // Update
				keys := reflect.ValueOf(kv[i]).MapKeys()
				if len(keys) == 0 {
					continue
				}
				key := []byte(keys[rand.Intn(len(keys))].Interface().(string))

				valLen := 1 + rand.Intn(64)
				val := make([]byte, valLen)
				rand.Read(val)

				kv[i][string(key)] = string(val)
				err := smt.Put(key, val)
				if err != nil {
					t.Errorf("error: %v", err)
				}
			} else { // Delete
				keys := reflect.ValueOf(kv[i]).MapKeys()
				if len(keys) == 0 {
					continue
				}
				key := []byte(keys[rand.Intn(len(keys))].Interface().(string))

				delete(kv[i], string(key))
				err := smt.Delete(key)
				if err != nil {
					t.Errorf("error: %v", err)
				}
			}

		}
		smt.Commit()
		smt.Discard()
		roots[i] = smt.Root()
		checkOne(t, smt, &kv[i], i)
	}
}

func checkOne(t *testing.T, smt store.SMTStore, kv *map[string]string, index int) {
	for k, v := range *kv {
		actualVal, err := smt.Get([]byte(k))
		if err != nil {
			t.Errorf("error: %v", err)
			continue
		}

		if !bytes.Equal([]byte(v), actualVal) {
			t.Errorf("got incorrect value when bulk testing blocks")
		}
	}
}

// func checkAll(t *testing.T, smt *SMTree, roots [][]byte, kv []map[string]string) {
// 	for i, root := range roots {
// 		for k, v := range kv[i] {
// 			actualVal, err := smt.GetFromRoot([]byte(k), root)
// 			if err != nil {
// 				t.Errorf("error: %v", err)
// 			}

// 			if !bytes.Equal([]byte(v), actualVal) {
// 				t.Error("got incorrect value when bulk testing blocks")
// 			}
// 		}
// 	}
// }
