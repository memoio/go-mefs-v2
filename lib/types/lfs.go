package types

import (
	"sync"

	rbtree "github.com/memoio/go-mefs-v2/lib/RbTree"
	mpb "github.com/memoio/go-mefs-v2/lib/pb"
	mt "gitlab.com/NebulousLabs/merkletree"
)

// superblock管理; 从leveldb中load
type superBlock struct {
	sync.RWMutex
	write          bool              // set true when finish unfinished jobs
	read           bool              // set true after loaded from kv
	bucketMax      uint64            // Get from chain; in case create too mant buckets
	bucketCount    uint64            // next bucket
	bucketIDToName map[string]uint64 // bucketName -> bucketID
	buckets        []bucketInfo      // 所有的bucket信息
}

// bucketInfo in memory 管理bucket信息
type bucketInfo struct {
	sync.RWMutex
	mpb.BucketOption
	mpb.BucketInfo                              // store in kv
	nextOpID       uint64                       // next op
	nextObjectID   uint64                       // next object
	offset         uint64                       // next
	payloads       map[uint64]*mpb.BucketRecord // key: opID,
	objects        *rbtree.Tree                 // store objects; key is object name
	mtree          *mt.Tree                     // update when done
}

// objectInfo in memory
type objectInfo struct {
	sync.RWMutex
	mpb.ObjectInfo
	deletion bool
	upload   bool // 记录当前object状态，upload
	parts    []mpb.PartInfo
}
