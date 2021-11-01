package keeper

import (
	"github.com/memoio/go-mefs-v2/lib/nodeinfo"
	mpb "github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
)

type LocalInfo struct {
	role    mpb.NodeInfo_RoleType
	localID uint64
	groupID uint64
}

// manage users
type UserManger struct {
	groupID uint64
	users   map[uint64]*nodeinfo.NodeInfo
}

// manage keepers in this group
type KeeperManger struct {
	groupID uint64
	keepers map[uint64]*nodeinfo.NodeInfo
}

// manage providers in this group
type ProManger struct {
	groupID uint64
	pros    map[uint64]*nodeinfo.NodeInfo
}

// manage fs, challenege and pay
type dataManage struct {
	groupID uint64
	segment types.SegMgr
	piece   types.PieceMgr
	sectors map[uint64]*types.SectorMgr // key: proID; value: *SectorInPro
	pay     types.PayMange
}
