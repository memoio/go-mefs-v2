package wallet

type WalletAPI struct { //nolint
	*Wallet
}

func (w *WalletAPI) List() ([]string, error) {
	return w.Wallet.List()
}
