package wallet

import "github.com/memoio/go-mefs-v2/api"

var _ api.IWallet = &walletAPI{}

type walletAPI struct {
	*LocalWallet
}
