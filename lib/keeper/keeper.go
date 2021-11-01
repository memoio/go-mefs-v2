package keeper

import (
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
)

type LocalInfo struct {
	role    pb.NodeInfo_RoleType
	localID uint64
	groupID uint64
}

// manage fs, challenege and pay
type dataManage struct {
	groupID uint64
	segment types.SegMgr
	piece   types.PieceMgr
	sectors map[uint64]*types.SectorMgr // key: proID; value: *SectorInPro
	pay     types.PayMange
}
