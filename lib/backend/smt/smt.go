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
	blkRoot, curRoot []byte // branches on / before blkRoot root will not be cleaned
	nodes, values    store.KVStore
}

func NewSMTree(root []byte, n store.KVStore, v store.KVStore) *SMTree {
	smtree := &SMTree{
		blkRoot: root,
		curRoot: root,
		nodes:   n,
		values:  &ValueStore{v},
	}
	if root == nil {
		tempTrie := smt.NewSparseMerkleTree(smtree.nodes, smtree.values, sha256.New())
		smtree.SetRoot(tempTrie.Root())
	}
	return smtree
}

// get latest root from trie
func (s *SMTree) Root() []byte {
	return s.curRoot
}

// should set root before a new round put to keep old branches
func (s *SMTree) SetRoot(root []byte) {
	s.blkRoot = root
	s.curRoot = root
}

// Rewind rewinds the sparse merkle tree to a new root. The method will try to remove discarded branches and leaves by old root and keys in the old-root tree. Note: makes sure that rewinds the root backwards.
func (s *SMTree) Rewind(newroot, oldroot []byte, keys [][]byte) error {
	err := s.CleanHistory(oldroot, keys)
	if err != nil {
		return err
	}

	// root rollback
	s.SetRoot(newroot)
	return nil
}

// CleanHistory cleans history data in the tree with old root.
// <oldroot>: target block's root,
// <keys>:
// 1. next block's inserted/updated keys (clean old history)
// 2. current block's inserted/updated keys (rollback)
// TODO: should consider same root case (as s.Put())
func (s *SMTree) CleanHistory(oldroot []byte, keys [][]byte) error {
	// a single operation share a common txn
	nTxn, err := s.nodes.NewTxnStore(true)
	if err != nil {
		return err
	}
	vTxn, err := s.values.NewTxnStore(true)
	if err != nil {
		return err
	}
	defer nTxn.Discard()
	defer vTxn.Discard()

	// new a tree
	trie := smt.ImportSparseMerkleTree(nTxn, vTxn, sha256.New(), oldroot)

	// cleans old-root tree
	err = trie.RemovePathsForRoot(keys, oldroot)
	return err
}

func (s *SMTree) Get(key []byte) ([]byte, error) {
	// a single operation share a common txn
	nTxn, err := s.nodes.NewTxnStore(false)
	if err != nil {
		return nil, err
	}
	vTxn, err := s.values.NewTxnStore(false)
	if err != nil {
		return nil, err
	}
	defer nTxn.Discard()
	defer vTxn.Discard()

	// new a tree
	trie := smt.ImportSparseMerkleTree(nTxn, vTxn, sha256.New(), s.curRoot)

	value, err := trie.Get(key)
	if bytes.Equal(value, emptyValue) {
		return nil, xerrors.Errorf("%s not found", string(key))
	}
	return value, err
}

func (s *SMTree) GetFromRoot(key, root []byte) ([]byte, error) {
	// a single operation share a common txn
	nTxn, err := s.nodes.NewTxnStore(false)
	if err != nil {
		return nil, err
	}
	vTxn, err := s.values.NewTxnStore(false)
	if err != nil {
		return nil, err
	}
	defer nTxn.Discard()
	defer vTxn.Discard()

	// new a tree
	trie := smt.ImportSparseMerkleTree(nTxn, vTxn, sha256.New(), s.curRoot)

	return trie.GetFromRoot(key, root)
}

func (s *SMTree) Put(key, value []byte) error {
	// a single operation share a common txn
	nTxn, err := s.nodes.NewTxnStore(true)
	if err != nil {
		return err
	}
	vTxn, err := s.values.NewTxnStore(true)
	if err != nil {
		return err
	}
	defer nTxn.Discard()
	defer vTxn.Discard()

	// new a tree
	trie := smt.ImportSparseMerkleTree(nTxn, vTxn, sha256.New(), s.curRoot)

	newRoot, err := trie.Update(key, value)
	if err != nil {
		// debug
		trie.PrintSMT(s.blkRoot)
		trie.PrintSMT(trie.Root())
		return xerrors.Errorf("smt put fail: %w", err)
	}

	// a meaningless put (same key & value)
	if bytes.Equal(s.curRoot, newRoot) {
		return nil
	}
	// preserve last block's root
	if !bytes.Equal(s.curRoot, s.blkRoot) {
		// intermediate root use to clean intermediate branch
		if err = trie.RemovePath(key, s.curRoot, s.blkRoot); err != nil {
			return err
		}
	}

	if err = nTxn.Commit(); err != nil {
		return err
	}
	if err = vTxn.Commit(); err != nil {
		return err
	}
	s.curRoot = newRoot
	return nil
}

func (s *SMTree) Has(key []byte) (bool, error) {
	// a single operation share a common txn
	nTxn, err := s.nodes.NewTxnStore(false)
	if err != nil {
		return false, err
	}
	vTxn, err := s.values.NewTxnStore(false)
	if err != nil {
		return false, err
	}
	defer nTxn.Discard()
	defer vTxn.Discard()

	// new a tree
	trie := smt.ImportSparseMerkleTree(nTxn, vTxn, sha256.New(), s.curRoot)

	return trie.Has(key)
}

func (s *SMTree) Delete(key []byte) error {
	// a single operation share a common txn
	nTxn, err := s.nodes.NewTxnStore(true)
	if err != nil {
		return err
	}
	vTxn, err := s.values.NewTxnStore(true)
	if err != nil {
		return err
	}
	defer nTxn.Discard()
	defer vTxn.Discard()

	// new a tree
	trie := smt.ImportSparseMerkleTree(nTxn, vTxn, sha256.New(), s.curRoot)

	newRoot, err := trie.Delete(key)
	if err != nil {
		return xerrors.Errorf("smt delete fail: %w", err)
	}

	// a meaningless delete (already empty)
	if bytes.Equal(s.curRoot, newRoot) {
		return err
	}
	if !bytes.Equal(s.curRoot, s.blkRoot) {
		if err = trie.RemovePath(key, s.curRoot, s.blkRoot); err != nil {
			return err
		}
	}

	if err = nTxn.Commit(); err != nil {
		return err
	}
	if err = vTxn.Commit(); err != nil {
		return err
	}
	s.curRoot = newRoot
	return nil
}

func (smt *SMTree) Size() store.DiskStats {
	nodeSize := smt.nodes.Size()
	return nodeSize
}

func (smt *SMTree) Close() error {
	if err := smt.nodes.Close(); err != nil {
		return err
	}
	if err := smt.values.Close(); err != nil {
		return err
	}
	return nil
}
