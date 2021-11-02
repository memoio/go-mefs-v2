package roleinfo

import (
	"context"
	"time"

	"github.com/memoio/go-mefs-v2/lib/pb"
)

type NodeApp interface {
	Get(uint64) (pb.RoleInfo, error)
	Put(uint64, pb.RoleInfo) error       // 添加或更新node
	Remove(uint64) error                 // 删除node
	List(pb.RoleInfo_Type) []pb.RoleInfo // 根据Type筛选
	Save()                               // 信息持久化，用于启动
	//metrics
}

type RoleMgr struct {
	roleID  uint64
	groupID uint64
	self    *pb.RoleInfo            // local node info
	infos   map[uint64]*pb.RoleInfo // get from chain
}

func New(groupID, roleID uint64) *RoleMgr {
	rm := &RoleMgr{
		roleID:  roleID,
		groupID: groupID,
	}
	return rm
}

func (rm *RoleMgr) Sync(ctx context.Context) {
	t := time.NewTicker(60 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			// load from chain
			rm.SyncFromChain(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (rm *RoleMgr) SyncFromChain(ctx context.Context) {

}
