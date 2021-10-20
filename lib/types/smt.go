package types

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"

	"github.com/celestiaorg/smt"
	"github.com/dgraph-io/badger/v2"
	"github.com/zeebo/blake3"
)

var (
	RootKey        = "Root"
	ConfirmRootKey = "ConfirmRoot"
	// key: root hash; val: applied key, previous root hash
	// 在清理确认块前的时候，用previous root hash 作为root清理key
	// 在回退的时候，用root hash作为root清理key
	PrevRootPrefix = "PrevRoot/"
	AppliedKey     = strconv.Itoa(1)
	KeySeq         = "KeySeq" // key seqnumber
	ApplyingPrefix = "Applying/"
)

var (
	ErrRoot = errors.New("root is not complete")
)

// todo: add clean up for state history
// need modify smt code
type statetree struct {
	*smt.SparseMerkleTree
	db     *stateDB
	height uint64
}

func NewStateTree(path string) (*statetree, error) {
	sdb, err := NewStateDB(path)
	if err != nil {
		return nil, err
	}

	root, err := sdb.Get([]byte(RootKey))
	if err != nil {
		// create new one
		tri := smt.NewSparseMerkleTree(sdb, sdb, blake3.New())
		return &statetree{tri, sdb, 0}, nil
	}

	if err == nil {
		fmt.Println("root is: ", root)
		_, err := sdb.Get(concat([]byte(PrevRootPrefix), root))
		if err != nil {
			return nil, ErrRoot
		}

		tri := smt.ImportSparseMerkleTree(sdb, sdb, blake3.New(), root)
		return &statetree{tri, sdb, 0}, nil
	}

	return nil, err
}

func (st *statetree) RunGC() error {
	return st.db.db.RunValueLogGC(0.5)
}

func (st *statetree) Close() error {
	return st.db.db.Close()
}

func (st *statetree) DB() *stateDB {
	return st.db
}

func (st *statetree) GetRoot() []byte {
	return st.Root()
}

func (st *statetree) SaveRoot() error {
	root := st.Root()
	return st.db.Set([]byte(RootKey), root)
}

func concat(vs ...[]byte) []byte {
	var b bytes.Buffer
	for _, v := range vs {
		b.Write(v)
	}
	return b.Bytes()
}

func (st *statetree) Put(key, value []byte) error {
	prevRoot := st.Root()

	root, err := st.Update(key, value)
	if err != nil {
		fmt.Println("put err: ", err)
		return err
	}

	if bytes.Equal(root, prevRoot) {
		fmt.Println("value is equal to old value")
		return nil
	}

	st.db.Set(concat([]byte(PrevRootPrefix), root), concat(prevRoot, key))
	return st.db.Set([]byte(RootKey), root)
}

func (st *statetree) Delete(key, root []byte) ([]byte, error) {
	return st.DeleteForRoot(key, root)
}

type stateDB struct {
	db         *badger.DB
	writeCount uint64
}

func NewStateDB(path string) (*stateDB, error) {
	ops := badger.DefaultOptions(path)
	db, err := badger.Open(ops)
	if err != nil {
		return nil, err
	}

	return &stateDB{db, 0}, nil
}

func (sd *stateDB) Get(key []byte) ([]byte, error) {
	var val []byte
	err := sd.db.View(func(txn *badger.Txn) error {
		switch item, err := txn.Get(key); err {
		case nil:
			val, err = item.ValueCopy(nil)
			return err
		default:
			return err
		}
	})
	if err != nil {
		return nil, err
	}
	return val, nil
}

func (sd *stateDB) GetCount() uint64 {
	return sd.writeCount
}

func (sd *stateDB) Set(key []byte, value []byte) error {
	err := sd.db.Update(func(txn *badger.Txn) error {
		err := txn.Set(key, value)
		return err
	})
	if err != nil {
		return err
	}
	sd.writeCount++
	return nil
}

func (sd *stateDB) Delete(key []byte) error {
	//fmt.Println("call delete: ", key)
	err := sd.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})
	return err
}
