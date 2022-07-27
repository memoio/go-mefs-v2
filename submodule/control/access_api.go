package control

import (
	"context"

	"github.com/memoio/go-mefs-v2/api"
)

var _ api.IRestrict = &accessAPI{}

type accessAPI struct {
	*wl
}

func (w *wl) RestrictStat(ctx context.Context) (bool, error) {
	return w.Stat(), nil
}
func (w *wl) RestrictEnable(ctx context.Context, ea bool) error {
	return w.Enable(ea)
}

func (w *wl) RestrictAdd(ctx context.Context, uid uint64) error {
	return w.Add(uid)
}
func (w *wl) RestrictDelete(ctx context.Context, uid uint64) error {
	return w.Delete(uid)
}
func (w *wl) RestrictHas(ctx context.Context, uid uint64) bool {
	return w.Has(uid)
}

func (w *wl) RestrictList(ctx context.Context) ([]uint64, error) {
	return w.List(), nil
}
