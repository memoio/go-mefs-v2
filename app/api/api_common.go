package api

import (
	"context"

	"github.com/filecoin-project/go-jsonrpc/auth"
)

type Common interface {
	// Auth
	AuthVerify(ctx context.Context, token string) ([]auth.Permission, error)
	AuthNew(ctx context.Context, perms []auth.Permission) ([]byte, error)
}
