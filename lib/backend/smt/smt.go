package smt

import (
	"bytes"
	"crypto/sha256"

	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/smt"
	"golang.org/x/xerrors"
)

var _ store.SMTStore = (*SMTree)(nil)

var emptyValue = []byte{}

type SMTree struct {
	root          []byte
	nodes, values store.KVStore
	nTxn, vTxn    store.TxnStore
	txnExist      bool
	smt           *smt.SparseMerkleTree
}

func NewSMTree(root []byte, n store.KVStore, v store.KVStore) *SMTree {
	smtree := &SMTree{
		root:     root,
		nodes:    n,
		values:   &ValueStore{v},
		txnExist: false,
	}
	s := smt.NewSparseMerkleTree(n, v, sha256.New())
	if root == nil {
		smtree.root = s.Root()
	} else {
		s.SetRoot(root)
	}
	smtree.smt = s
	return smtree
}

func (smt *SMTree) Root() []byte {
	return smt.root
}

func (smt *SMTree) SetRoot(root []byte) {
	smt.root = root
	smt.smt.SetRoot(root)
}

// Rewind rewinds the sparse merkle tree to a new root. The method will try to remove discarded branches and leaves by old root and keys in the old-root tree. Note: makes sure that rewinds the root backwards.
func (smt *SMTree) Rewind(newroot, oldroot []byte, keys [][]byte) error {
	err := smt.CleanHistory(oldroot, keys)
	if err != nil {
		return err
	}

	// root rollback
	smt.SetRoot(newroot)
	return nil
}

// CleanHistory cleans history data in the tree with old root.
// <oldroot>: target block's root,
// <keys>:
// 1. next block's inserted/updated keys (clean old history)
// 2. current block's inserted/updated keys (rollback)
// TODO: should consider same root case (as smt.Put())
func (smt *SMTree) CleanHistory(oldroot []byte, keys [][]byte) error {
	// if txn exists, discards it to avoid inconsistency
	if smt.txnExist {
		smt.Discard()
	}

	smt.NewTxn()
	defer smt.Discard()

	// cleans old-root tree
	err := smt.smt.RemovePathsForRoot(keys, oldroot)
	if err != nil {
		return err
	}
	smt.Commit()
	return nil
}

func (smt *SMTree) NewTxn() error {
	var err error
	smt.nTxn, err = smt.nodes.NewTxnStore(true)
	if err != nil {
		return err
	}
	smt.vTxn, err = smt.values.NewTxnStore(true)
	if err != nil {
		return err
	}
	smt.txnExist = true
	smt.smt.SetStore(smt.nTxn, smt.vTxn, smt.root)
	return nil
}

func (smt *SMTree) Get(key []byte) ([]byte, error) {
	if !smt.txnExist { // get value from KVStore
		// set kvstore for smt's mapstore
		smt.smt.SetStore(smt.nodes, smt.values, smt.root)
	}
	value, err := smt.smt.Get(key)
	if bytes.Equal(value, emptyValue) {
		return nil, xerrors.Errorf("%s not found", string(key))
	}
	return value, err
}

func (smt *SMTree) GetFromRoot(key, root []byte) ([]byte, error) {
	if !smt.txnExist { // get value from KVStore
		// set kvstore for smt's mapstore
		smt.smt.SetStore(smt.nodes, smt.values, smt.root)
	}
	return smt.smt.GetFromRoot(key, root)
}

func (smt *SMTree) Put(key, value []byte) error {
	if !smt.txnExist {
		err := smt.NewTxn()
		if err != nil {
			return err
		}
	}

	// intermediate root use to clean intermediate branch
	oldRoot := smt.smt.Root()
	_, err := smt.smt.Update(key, value)
	if err != nil {
		// debug
		smt.smt.PrintSMT(smt.root)
		smt.smt.PrintSMT(smt.smt.Root())
		return xerrors.Errorf("smt put fail: %w", err)
	}

	// a meaningless put (same key & value)
	if bytes.Equal(oldRoot, smt.smt.Root()) {
		return nil
	}
	// preserve last block's root
	if !bytes.Equal(oldRoot, smt.root) {
		err = smt.smt.RemovePath(key, oldRoot, smt.root)
	}
	return err
}

func (smt *SMTree) Has(key []byte) (bool, error) {
	if !smt.txnExist {
		smt.smt.SetStore(smt.nodes, smt.values, smt.root)
	}
	return smt.smt.Has(key)
}

func (smt *SMTree) Delete(key []byte) error {
	if !smt.txnExist {
		err := smt.NewTxn()
		if err != nil {
			return err
		}
	}

	oldRoot := smt.smt.Root()
	_, err := smt.smt.Delete(key)
	if err != nil {
		return xerrors.Errorf("smt delete fail: %w", err)
	}

	// a meaningless delete (already empty)
	if bytes.Equal(oldRoot, smt.smt.Root()) {
		return err
	}
	if !bytes.Equal(oldRoot, smt.root) {
		err = smt.smt.RemovePath(key, oldRoot, smt.root)
	}
	return err
}

func (smt *SMTree) Size() store.DiskStats {
	nodeSize := smt.nodes.Size()
	return nodeSize
}

func (smt *SMTree) Close() error {
	err := smt.nodes.Close()
	if err != nil {
		return err
	}
	err = smt.values.Close()
	if err != nil {
		return err
	}
	smt.smt = nil
	return nil
}

func (smt *SMTree) Commit() error {
	if !smt.txnExist {
		return nil
	}
	err := smt.nTxn.Commit()
	if err != nil {
		return err
	}
	err = smt.vTxn.Commit()
	if err != nil {
		return err
	}
	// save new root in memory
	smt.root = smt.smt.Root()
	return nil
}

func (smt *SMTree) Discard() {
	if !smt.txnExist {
		return
	}
	smt.nTxn.Discard()
	smt.vTxn.Discard()
	smt.txnExist = false
}
