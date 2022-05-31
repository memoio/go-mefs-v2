package impl

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/memoio/contractsv2/go_contracts/getter"
	"github.com/memoio/contractsv2/go_contracts/role"
	inter "github.com/memoio/go-mefs-v2/submodule/connect/v2/interface"
)

type getImpl struct {
	endPoint string
	chainID  *big.Int

	sk string

	eAddr   common.Address
	getAddr common.Address
}

func NewGetter(endPoint, hexSk string, getAddr common.Address) (inter.IGetter, error) {
	client, err := ethclient.DialContext(context.TODO(), endPoint)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	chainID, err := client.NetworkID(context.Background())
	if err != nil {
		chainID = big.NewInt(666)
	}

	eAddr, err := SkToAddr(hexSk)
	if err != nil {
		return nil, err
	}

	// check erc20 is contract
	getIns, err := getter.NewGetter(getAddr, client)
	if err != nil {
		return nil, err
	}

	_, err = getIns.Name(&bind.CallOpts{
		From: eAddr,
	})
	if err != nil {
		return nil, err
	}

	g := &getImpl{
		endPoint: endPoint,
		chainID:  chainID,
		sk:       hexSk,
		eAddr:    eAddr,
		getAddr:  getAddr,
	}

	return g, nil
}

func (g *getImpl) GetRoleInfo(addr common.Address) (*role.RoleOut, error) {
	return nil, nil
}
