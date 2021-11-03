package config

import "github.com/memoio/go-mefs-v2/api"

var _ api.IConfig = &configAPI{}

type configAPI struct { //nolint
	*ConfigModule
}
