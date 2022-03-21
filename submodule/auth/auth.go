package auth

import (
	"context"
	"crypto/rand"
	"io"
	"io/ioutil"

	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/gbrlsnchs/jwt/v3"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/repo"
	"github.com/memoio/go-mefs-v2/lib/types"
)

const (
	jwtSecetName = "auth-jwt"
)

type JwtAuth struct {
	APISecret *jwt.HMACSHA
}

type jwtPayload struct {
	Allow []auth.Permission
}

func NewJwtAuth(rp repo.Repo) (*JwtAuth, error) {
	key, err := rp.KeyStore().Get(jwtSecetName, jwtSecetName)
	if err != nil {
		sk, err := ioutil.ReadAll(io.LimitReader(rand.Reader, 32))
		if err != nil {
			return nil, err
		}
		key = types.KeyInfo{
			Type:      types.Hmac,
			SecretKey: sk,
		}

		err = rp.KeyStore().Put(jwtSecetName, jwtSecetName, key)
		if err != nil {
			return nil, xerrors.Errorf("writing API secret: %w", err)
		}

		p := jwtPayload{
			Allow: api.AllPermissions,
		}

		cliToken, err := jwt.Sign(&p, jwt.NewHS256(key.SecretKey))
		if err != nil {
			return nil, err
		}

		err = rp.SetAPIToken(cliToken)
		if err != nil {
			return nil, err
		}
	}

	return &JwtAuth{
		jwt.NewHS256(key.SecretKey),
	}, nil
}

func (a *JwtAuth) API() *authAPI {
	return &authAPI{a}
}

func (a *JwtAuth) AuthVerify(ctx context.Context, token string) ([]auth.Permission, error) {
	var payload jwtPayload
	if _, err := jwt.Verify([]byte(token), (*jwt.HMACSHA)(a.APISecret), &payload); err != nil {
		return nil, xerrors.Errorf("JWT Verification failed: %w", err)
	}

	return payload.Allow, nil
}

func (a *JwtAuth) AuthNew(ctx context.Context, perms []auth.Permission) ([]byte, error) {
	p := jwtPayload{
		Allow: perms, // TODO: consider checking validity
	}

	return jwt.Sign(&p, (*jwt.HMACSHA)(a.APISecret))
}
