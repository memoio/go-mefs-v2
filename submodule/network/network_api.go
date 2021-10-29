package network

import (
	"github.com/memoio/go-mefs-v2/app/api"
)

var _ api.INetwork = &networkAPI{}

type networkAPI struct { //nolint
	*NetworkSubmodule
}
