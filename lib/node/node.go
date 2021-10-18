package node

import (
	"math/big"

	mpb "github.com/memoio/go-mefs-v2/lib/pb"
)

type NodeInfo struct {
	nInfo    mpb.NodeInfo
	gasTotal *big.Int // from chain
	gasUsed  *big.Int // money used for gas
	pin      bool     // belongs to localID
	netID    string
}

type NodeMgr struct {
	local uint64
	nodes map[uint64]*NodeInfo // key: nodeID
}

type NodeApp interface {
	Get(uint64) (NodeInfo, error)
	Put(uint64, NodeInfo) error             // 添加或更新node
	Remove(uint64) error                    // 删除node
	List(mpb.NodeInfo_RoleType) []*NodeInfo // 根据Type筛选
	Save()                                  // 信息持久化，用于启动
	//metrics
}
