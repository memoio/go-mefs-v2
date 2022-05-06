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
	root          []byte // keep_root, branches on / before this root will not be cleaned
	nodes, values store.KVStore
	nTxn, vTxn    store.TxnStore
	smt           *smt.SparseMerkleTree
}

func NewSMTree(root []byte, n store.KVStore, v store.KVStore) *SMTree {
	smtree := &SMTree{
		root:   root,
		nodes:  n,
		values: &ValueStore{v},
	}
	s := smt.NewSparseMerkleTree(smtree.nodes, smtree.values, sha256.New())
	if root == nil {
		smtree.root = s.Root()
	} else {
		s.SetRoot(root)
	}
	smtree.smt = s
	return smtree
}

// get latest root from trie
func (smt *SMTree) Root() []byte {
	return smt.smt.Root()
}

// should set root before a new round put to keep old branches
func (smt *SMTree) SetRoot(root []byte) {
	if root == nil {
		smt.root = smt.smt.Root()
	} else {
		smt.root = root
		smt.smt.SetRoot(root)
	}
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
	// cleans old-root tree
	err := smt.smt.RemovePathsForRoot(keys, oldroot)
	if err != nil {
		return err
	}
	return nil
}

// a single operation share a common txn
func (smt *SMTree) newTxn(update bool) error {
	var err error
	smt.nTxn, err = smt.nodes.NewTxnStore(update)
	if err != nil {
		return err
	}
	smt.vTxn, err = smt.values.NewTxnStore(update)
	if err != nil {
		return err
	}
	smt.smt.SetStore(smt.nTxn, smt.vTxn, smt.smt.Root())
	return nil
}

func (smt *SMTree) commitTxn() error {
	err := smt.nTxn.Commit()
	if err != nil {
		return err
	}
	err = smt.vTxn.Commit()
	if err != nil {
		return err
	}
	return nil
}

func (smt *SMTree) discardTxn() {
	smt.nTxn.Discard()
	smt.vTxn.Discard()
}

func (smt *SMTree) Get(key []byte) ([]byte, error) {
	if err := smt.newTxn(false); err != nil {
		return nil, err
	}
	defer smt.discardTxn()

	value, err := smt.smt.Get(key)
	if bytes.Equal(value, emptyValue) {
		return nil, xerrors.Errorf("%s not found", string(key))
	}
	return value, err
}

func (smt *SMTree) GetFromRoot(key, root []byte) ([]byte, error) {
	if err := smt.newTxn(false); err != nil {
		return nil, err
	}
	defer smt.discardTxn()

	return smt.smt.GetFromRoot(key, root)
}

func (smt *SMTree) Put(key, value []byte) error {
	if err := smt.newTxn(true); err != nil {
		return err
	}
	defer smt.discardTxn()

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
		if err = smt.smt.RemovePath(key, oldRoot, smt.root); err != nil {
			return err
		}
	}

	err = smt.commitTxn()
	return err
}

func (smt *SMTree) Has(key []byte) (bool, error) {
	if err := smt.newTxn(false); err != nil {
		return false, err
	}
	defer smt.discardTxn()

	return smt.smt.Has(key)
}

func (smt *SMTree) Delete(key []byte) error {
	if err := smt.newTxn(true); err != nil {
		return err
	}
	defer smt.discardTxn()

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
		if err = smt.smt.RemovePath(key, oldRoot, smt.root); err != nil {
			return err
		}
	}

	err = smt.commitTxn()
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
