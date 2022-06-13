package impl

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/memoio/contractsv2/go_contracts/getter"
	"github.com/memoio/go-mefs-v2/api"
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

	_, err = getIns.Version(&bind.CallOpts{
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

// token related
func (g *getImpl) GetToken(ti uint8) (common.Address, error) {
	var res common.Address
	client, err := ethclient.DialContext(context.TODO(), g.endPoint)
	if err != nil {
		return res, err
	}
	defer client.Close()

	getIns, err := getter.NewGetter(g.getAddr, client)
	if err != nil {
		return res, err
	}

	retryCount := 0
	for {
		retryCount++
		res, err := getIns.GetTA(&bind.CallOpts{
			From: g.eAddr,
		}, ti)
		if err != nil {
			if retryCount > 3 {
				return res, err
			}
			time.Sleep(5 * time.Second)
			continue
		}

		return res, nil
	}
}

// role Related
func (g *getImpl) GetAddrCnt() uint64 {
	client, err := ethclient.DialContext(context.TODO(), g.endPoint)
	if err != nil {
		return 0
	}
	defer client.Close()

	getIns, err := getter.NewGetter(g.getAddr, client)
	if err != nil {
		return 0
	}

	retryCount := 0
	for {
		retryCount++
		cnt, err := getIns.GetACnt(&bind.CallOpts{
			From: g.eAddr,
		})
		if err != nil {
			if retryCount > 3 {
				return 0
			}
			time.Sleep(5 * time.Second)
			continue
		}

		return cnt
	}
}

func (g *getImpl) GetAddrAt(i uint64) (common.Address, error) {
	var res common.Address
	client, err := ethclient.DialContext(context.TODO(), g.endPoint)
	if err != nil {
		return res, err
	}
	defer client.Close()

	getIns, err := getter.NewGetter(g.getAddr, client)
	if err != nil {
		return res, err
	}

	retryCount := 0
	for {
		retryCount++
		res, err := getIns.GetAddr(&bind.CallOpts{
			From: g.eAddr,
		}, i)
		if err != nil {
			if retryCount > 3 {
				return res, err
			}
			time.Sleep(5 * time.Second)
			continue
		}

		return res, nil
	}
}

func (g *getImpl) GetRoleInfo(addr common.Address) (*getter.RoleOut, error) {
	client, err := ethclient.DialContext(context.TODO(), g.endPoint)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	getIns, err := getter.NewGetter(g.getAddr, client)
	if err != nil {
		return nil, err
	}

	retryCount := 0
	for {
		retryCount++
		res, err := getIns.GetRInfo(&bind.CallOpts{
			From: g.eAddr,
		}, addr)
		if err != nil {
			if retryCount > 3 {
				return nil, err
			}
			time.Sleep(5 * time.Second)
			continue
		}

		return &res, nil
	}
}

func (g *getImpl) GetGroupInfo(gi uint64) (*api.GroupInfo, error) {
	client, err := ethclient.DialContext(context.TODO(), g.endPoint)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	getIns, err := getter.NewGetter(g.getAddr, client)
	if err != nil {
		return nil, err
	}

	isActive, isBanned, err := getIns.GetGInfo(&bind.CallOpts{
		From: g.eAddr,
	}, gi)
	if err != nil {
		return nil, err
	}

	level, err := getIns.GetLevel(&bind.CallOpts{
		From: g.eAddr,
	}, gi)
	if err != nil {
		return nil, err
	}

	kpr, ppr, err := getIns.GetPInfo(&bind.CallOpts{
		From: g.eAddr,
	}, gi)
	if err != nil {
		return nil, err
	}

	km, err := getIns.GetKManage(&bind.CallOpts{
		From: g.eAddr,
	}, gi)
	if err != nil {
		return nil, err
	}

	kcnt, err := getIns.GetKCnt(&bind.CallOpts{
		From: g.eAddr,
	}, gi)
	if err != nil {
		return nil, err
	}

	gout, err := getIns.GetGSInfo(&bind.CallOpts{
		From: g.eAddr,
	}, gi, 0)
	if err != nil {
		return nil, err
	}

	ginfo := &api.GroupInfo{
		EndPoint: g.endPoint,
		IsActive: isActive,
		IsBan:    isBanned,
		Level:    level,
		Kpr:      kpr,
		Ppr:      ppr,
		FsAddr:   km.String(),
		KCount:   uint64(kcnt),
		Size:     gout.Size,
		Price:    gout.Sprice,
	}

	return ginfo, nil
}

// pledge related
func (g *getImpl) GetPledgePool() (common.Address, error) {
	var res common.Address
	client, err := ethclient.DialContext(context.TODO(), g.endPoint)
	if err != nil {
		return res, err
	}
	defer client.Close()

	getIns, err := getter.NewGetter(g.getAddr, client)
	if err != nil {
		return res, err
	}

	retryCount := 0
	for {
		retryCount++
		res, err := getIns.Instances(&bind.CallOpts{
			From: g.eAddr,
		}, 5)
		if err != nil {
			if retryCount > 3 {
				return res, err
			}
			time.Sleep(5 * time.Second)
			continue
		}

		return res, nil
	}
}

func (g *getImpl) GetTotalPledge() *big.Int {
	res := new(big.Int)
	client, err := ethclient.DialContext(context.TODO(), g.endPoint)
	if err != nil {
		return res
	}
	defer client.Close()

	getIns, err := getter.NewGetter(g.getAddr, client)
	if err != nil {
		return res
	}

	retryCount := 0
	for {
		retryCount++
		res, err := getIns.GetTotalPledge(&bind.CallOpts{
			From: g.eAddr,
		})
		if err != nil {
			if retryCount > 3 {
				return res
			}
			time.Sleep(5 * time.Second)
			continue
		}

		return res
	}
}

func (g *getImpl) GetPledge(ti uint8) *big.Int {
	res := new(big.Int)
	client, err := ethclient.DialContext(context.TODO(), g.endPoint)
	if err != nil {
		return res
	}
	defer client.Close()

	getIns, err := getter.NewGetter(g.getAddr, client)
	if err != nil {
		return res
	}

	retryCount := 0
	for {
		retryCount++
		res, err := getIns.GetPledge(&bind.CallOpts{
			From: g.eAddr,
		}, ti)
		if err != nil {
			if retryCount > 3 {
				return res
			}
			time.Sleep(5 * time.Second)
			continue
		}

		return res
	}
}

func (g *getImpl) GetPledgeAt(i uint64, ti uint8) *big.Int {
	res := new(big.Int)
	client, err := ethclient.DialContext(context.TODO(), g.endPoint)
	if err != nil {
		return res
	}
	defer client.Close()

	getIns, err := getter.NewGetter(g.getAddr, client)
	if err != nil {
		return res
	}

	retryCount := 0
	for {
		retryCount++
		res, err := getIns.PleBalanceOf(&bind.CallOpts{
			From: g.eAddr,
		}, i, ti)
		if err != nil {
			if retryCount > 3 {
				return res
			}
			time.Sleep(5 * time.Second)
			continue
		}

		return res
	}
}

// fs related
func (g *getImpl) GetFsPool() (common.Address, error) {
	var res common.Address
	client, err := ethclient.DialContext(context.TODO(), g.endPoint)
	if err != nil {
		return res, err
	}
	defer client.Close()

	getIns, err := getter.NewGetter(g.getAddr, client)
	if err != nil {
		return res, err
	}

	retryCount := 0
	for {
		retryCount++
		res, err := getIns.Instances(&bind.CallOpts{
			From: g.eAddr,
		}, 12)
		if err != nil {
			if retryCount > 3 {
				return res, err
			}
			time.Sleep(5 * time.Second)
			continue
		}

		return res, nil
	}
}

func (g *getImpl) GetBalAt(i uint64, ti uint8) (*big.Int, *big.Int) {
	res := new(big.Int)
	lock := new(big.Int)
	client, err := ethclient.DialContext(context.TODO(), g.endPoint)
	if err != nil {
		return res, lock
	}
	defer client.Close()

	getIns, err := getter.NewGetter(g.getAddr, client)
	if err != nil {
		return res, lock
	}

	retryCount := 0
	for {
		retryCount++
		res, lock, err = getIns.FsBalanceOf(&bind.CallOpts{
			From: g.eAddr,
		}, i, ti)
		if err != nil {
			if retryCount > 3 {
				return res, lock
			}
			time.Sleep(5 * time.Second)
			continue
		}

		return res, lock
	}
}

func (g *getImpl) GetStoreInfo(ui, pi uint64, ti uint8) *getter.StoreOut {
	client, err := ethclient.DialContext(context.TODO(), g.endPoint)
	if err != nil {
		return nil
	}
	defer client.Close()

	getIns, err := getter.NewGetter(g.getAddr, client)
	if err != nil {
		return nil
	}

	retryCount := 0
	for {
		retryCount++
		res, err := getIns.GetStoreInfo(&bind.CallOpts{
			From: g.eAddr,
		}, ui, pi, ti)
		if err != nil {
			if retryCount > 3 {
				return nil
			}
			time.Sleep(5 * time.Second)
			continue
		}

		return &res
	}
}

func (g *getImpl) GetSettleInfo(pi uint64, ti uint8) *getter.SettleOut {
	client, err := ethclient.DialContext(context.TODO(), g.endPoint)
	if err != nil {
		return nil
	}
	defer client.Close()

	getIns, err := getter.NewGetter(g.getAddr, client)
	if err != nil {
		return nil
	}

	retryCount := 0
	for {
		retryCount++
		res, err := getIns.GetSettleInfo(&bind.CallOpts{
			From: g.eAddr,
		}, pi, ti)
		if err != nil {
			if retryCount > 3 {
				return nil
			}
			time.Sleep(5 * time.Second)
			continue
		}

		return &res
	}
}

func (g *getImpl) GetFsInfo(ui, pi uint64) *getter.FsOut {
	client, err := ethclient.DialContext(context.TODO(), g.endPoint)
	if err != nil {
		return nil
	}
	defer client.Close()

	getIns, err := getter.NewGetter(g.getAddr, client)
	if err != nil {
		return nil
	}

	retryCount := 0
	for {
		retryCount++
		res, err := getIns.GetFsInfo(&bind.CallOpts{
			From: g.eAddr,
		}, ui, pi)
		if err != nil {
			if retryCount > 3 {
				return nil
			}
			time.Sleep(5 * time.Second)
			continue
		}

		return &res
	}
}

func (g *getImpl) GetGInfo(gi uint64, ti uint8) *getter.GroupOut {
	client, err := ethclient.DialContext(context.TODO(), g.endPoint)
	if err != nil {
		return nil
	}
	defer client.Close()

	getIns, err := getter.NewGetter(g.getAddr, client)
	if err != nil {
		return nil
	}

	retryCount := 0
	for {
		retryCount++
		res, err := getIns.GetGSInfo(&bind.CallOpts{
			From: g.eAddr,
		}, gi, ti)
		if err != nil {
			if retryCount > 3 {
				return nil
			}
			time.Sleep(5 * time.Second)
			continue
		}

		return &res
	}
}
