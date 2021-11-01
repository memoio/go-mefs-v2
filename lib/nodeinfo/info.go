package nodeinfo

import (
	"github.com/memoio/go-mefs-v2/lib/pb"
)

type NodeApp interface {
	Get(uint64) (pb.NodeInfo, error)
	Put(uint64, pb.NodeInfo) error           // 添加或更新node
	Remove(uint64) error                     // 删除node
	List(pb.NodeInfo_RoleType) []pb.NodeInfo // 根据Type筛选
	Save()                                   // 信息持久化，用于启动
	//metrics
}

type NodeMgr struct {
	nodeID   uint64
	groupID  uint64
	nodeInfo *pb.NodeInfo            // local node info
	infos    map[uint64]*pb.NodeInfo // get from chain
}

func New(groupID, nodeID uint64) *NodeMgr {
	nm := &NodeMgr{
		nodeID:  nodeID,
		groupID: groupID,
	}
	return nm
}
