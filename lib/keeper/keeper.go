package keeper

import (
	"github.com/memoio/go-mefs-v2/lib/types"
)

// manage fs, challenege and pay
type dataManage struct {
	groupID uint64
	segment types.SegMgr
	piece   types.PieceMgr
	sectors map[uint64]*types.SectorMgr // key: proID; value: *SectorInPro
	pay     types.PayMange
}
