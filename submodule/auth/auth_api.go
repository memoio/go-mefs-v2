package auth

import "github.com/memoio/go-mefs-v2/api"

var _ api.IAuth = &authAPI{}

type authAPI struct {
	*JwtAuth
}
