package impl

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/memoio/contractsv2/v2/getter"
	"github.com/memoio/go-mefs-v2/api"
	scom "github.com/memoio/go-mefs-v2/submodule/connect/settle/common"
	inter "github.com/memoio/go-mefs-v2/submodule/connect/settle/interface"
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

	eAddr, err := scom.SkToAddr(hexSk)
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

func (g *getImpl) GetRoleInfo(addr common.Address) (*inter.RoleOut, error) {
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

		return &inter.RoleOut{
			State:        res.State,
			RType:        res.RType,
			Index:        res.Index,
			GIndex:       res.GIndex,
			RegisterTime: big.NewInt(0),
			Owner:        res.Owner,
			Next:         res.Next,
			VerifyKey:    res.VerifyKey,
			Desc:         res.Desc,
		}, nil
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

	state, err := getIns.GetGS(&bind.CallOpts{
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

	/*
		// fix: too many keeper
		if level < (kcnt+1)*2/3 {
			level = (kcnt + 1) * 2 / 3
		}
	*/

	kpB, ppB, err := getIns.GetPlePerB(&bind.CallOpts{
		From: g.eAddr,
	}, gi)
	if err != nil {
		return nil, err
	}

	ginfo := &api.GroupInfo{
		EndPoint: g.endPoint,
		State:    state,
		Level:    level,
		Kpr:      kpr,
		Ppr:      ppr,
		KCount:   uint64(kcnt),
		Size:     gout.Size,
		Price:    gout.Sprice,
		KpB:      kpB,
		PpB:      ppB,
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

func (g *getImpl) GetTotalPledge() (*big.Int, error) {
	res := new(big.Int)
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
		res, err := getIns.GetTotalPledge(&bind.CallOpts{
			From: g.eAddr,
		})
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

func (g *getImpl) GetPledge(ti uint8) (*big.Int, error) {
	res := new(big.Int)
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
		res, err := getIns.GetPledge(&bind.CallOpts{
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

func (g *getImpl) GetPledgeAt(i uint64, ti uint8) (*big.Int, error) {
	res := new(big.Int)
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
		res, err := getIns.PleBalanceOf(&bind.CallOpts{
			From: g.eAddr,
		}, i, ti)
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

func (g *getImpl) GetBalAt(i uint64, ti uint8) (*big.Int, *big.Int, *big.Int, error) {
	fsBalance := new(big.Int)
	lock := new(big.Int)
	penalty := new(big.Int)
	client, err := ethclient.DialContext(context.TODO(), g.endPoint)
	if err != nil {
		return fsBalance, lock, penalty, err
	}
	defer client.Close()

	getIns, err := getter.NewGetter(g.getAddr, client)
	if err != nil {
		return fsBalance, lock, penalty, err
	}

	retryCount := 0
	for {
		retryCount++
		fsBalance, lock, penalty, err = getIns.FsBalanceOf(&bind.CallOpts{
			From: g.eAddr,
		}, i, ti)
		if err != nil {
			if retryCount > 3 {
				return fsBalance, lock, penalty, err
			}
			time.Sleep(5 * time.Second)
			continue
		}

		//res.Sub(res, penalty)

		return fsBalance, lock, penalty, err
	}
}

func (g *getImpl) GetStoreInfo(ui, pi uint64, ti uint8) (*inter.StoreOut, error) {
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
		res, err := getIns.GetStoreInfo(&bind.CallOpts{
			From: g.eAddr,
		}, ui, pi, ti)
		if err != nil {
			if retryCount > 3 {
				return nil, err
			}
			time.Sleep(5 * time.Second)
			continue
		}

		return &inter.StoreOut{
			Start:  res.Start,
			End:    res.End,
			Size:   res.Size,
			Sprice: res.Sprice,
		}, err
	}
}

func (g *getImpl) GetSettleInfo(pi uint64, ti uint8) (*inter.SettleOut, error) {
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
		res, err := getIns.GetSettleInfo(&bind.CallOpts{
			From: g.eAddr,
		}, pi, ti)
		if err != nil {
			if retryCount > 3 {
				return nil, err
			}
			time.Sleep(5 * time.Second)
			continue
		}

		return &inter.SettleOut{
			Time:    res.Time,
			Size:    res.Size,
			Price:   res.Price,
			MaxPay:  res.MaxPay,
			HasPaid: res.HasPaid,
			CanPay:  res.CanPay,
			Lost:    res.Lost,
		}, nil
	}
}

func (g *getImpl) GetFsInfo(ui, pi uint64) (*inter.FsOut, error) {
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
		res, err := getIns.GetFsInfo(&bind.CallOpts{
			From: g.eAddr,
		}, ui, pi)
		if err != nil {
			if retryCount > 3 {
				return nil, err
			}
			time.Sleep(5 * time.Second)
			continue
		}

		return &inter.FsOut{
			Nonce:    res.Nonce,
			SubNonce: res.SubNonce,
		}, nil
	}
}

func (g *getImpl) GetGInfo(gi uint64, ti uint8) (*inter.GroupOut, error) {
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
		res, err := getIns.GetGSInfo(&bind.CallOpts{
			From: g.eAddr,
		}, gi, ti)
		if err != nil {
			if retryCount > 3 {
				return nil, err
			}
			time.Sleep(5 * time.Second)
			continue
		}

		return &inter.GroupOut{
			Size:   res.Size,
			Sprice: res.Sprice,
			Lost:   res.Lost,
		}, nil
	}
}

func (g *getImpl) GetPleRewardInfo(index uint64, ti uint8) (*inter.RewardOut, error) {
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
		accu, last, pledge, reward, err := getIns.Rewards(&bind.CallOpts{
			From: g.eAddr,
		}, index, ti)
		if err != nil {
			if retryCount > 3 {
				return nil, err
			}
			time.Sleep(5 * time.Second)
			continue
		}

		return &inter.RewardOut{
			Accu:       accu,
			Last:       last,
			Pledge:     pledge,
			Reward:     reward,
			CurReward:  big.NewInt(0),
			PledgeTime: big.NewInt(0),
		}, nil
	}
}
