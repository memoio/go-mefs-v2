package role

import (
	"github.com/memoio/go-mefs-v2/app/api"
)

var _ api.IRole = &roleAPI{}

type roleAPI struct {
	*RoleMgr
}
