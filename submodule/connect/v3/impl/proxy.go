package impl

import (
	"context"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/memoio/contractsv2/go_contracts/proxy"
	scom "github.com/memoio/go-mefs-v2/submodule/connect/settle/common"
	inter "github.com/memoio/go-mefs-v2/submodule/connect/settle/interface"
)

type proxyImpl struct {
	endPoint string
	chainID  *big.Int

	sk string

	eAddr common.Address
	proxy common.Address
}

func NewProxy(endPoint, hexSk string, proxyAddr common.Address) (inter.IProxy, error) {
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
	proxyIns, err := proxy.NewProxy(proxyAddr, client)
	if err != nil {
		return nil, err
	}

	_, err = proxyIns.Version(&bind.CallOpts{
		From: eAddr,
	})
	if err != nil {
		return nil, err
	}

	p := &proxyImpl{
		endPoint: endPoint,
		chainID:  chainID,
		sk:       hexSk,
		eAddr:    eAddr,
		proxy:    proxyAddr,
	}

	return p, nil
}

func (p *proxyImpl) ReAcc() error {
	client, err := ethclient.DialContext(context.TODO(), p.endPoint)
	if err != nil {
		return err
	}
	defer client.Close()

	proxyIns, err := proxy.NewProxy(p.proxy, client)
	if err != nil {
		return err
	}

	auth, err := scom.MakeAuth(p.chainID, p.sk)
	if err != nil {
		return err
	}

	tx, err := proxyIns.ReAcc(auth)
	if err != nil {
		return err
	}

	return scom.CheckTx(p.endPoint, tx, "register account")
}

func (p *proxyImpl) ReRole(rtype uint8, extra []byte) error {
	client, err := ethclient.DialContext(context.TODO(), p.endPoint)
	if err != nil {
		return err
	}
	defer client.Close()

	proxyIns, err := proxy.NewProxy(p.proxy, client)
	if err != nil {
		return err
	}

	auth, err := scom.MakeAuth(p.chainID, p.sk)
	if err != nil {
		return err
	}

	tx, err := proxyIns.ReRole(auth, rtype, extra)
	if err != nil {
		return err
	}

	return scom.CheckTx(p.endPoint, tx, "register role")
}

func (p *proxyImpl) QuitRole(rid uint64) error {
	client, err := ethclient.DialContext(context.TODO(), p.endPoint)
	if err != nil {
		return err
	}
	defer client.Close()

	proxyIns, err := proxy.NewProxy(p.proxy, client)
	if err != nil {
		return err
	}

	auth, err := scom.MakeAuth(p.chainID, p.sk)
	if err != nil {
		return err
	}

	tx, err := proxyIns.QuitRole(auth, rid)
	if err != nil {
		return err
	}

	return scom.CheckTx(p.endPoint, tx, "quite role")
}

func (p *proxyImpl) AlterPayee(rid uint64, np common.Address) error {
	client, err := ethclient.DialContext(context.TODO(), p.endPoint)
	if err != nil {
		return err
	}
	defer client.Close()

	proxyIns, err := proxy.NewProxy(p.proxy, client)
	if err != nil {
		return err
	}

	auth, err := scom.MakeAuth(p.chainID, p.sk)
	if err != nil {
		return err
	}

	tx, err := proxyIns.AlterPayee(auth, rid, np)
	if err != nil {
		return err
	}

	return scom.CheckTx(p.endPoint, tx, "alter payee")
}

// add a user/keeper/provider to group
func (p *proxyImpl) AddToGroup(gi uint64) error {
	client, err := ethclient.DialContext(context.TODO(), p.endPoint)
	if err != nil {
		return err
	}
	defer client.Close()

	proxyIns, err := proxy.NewProxy(p.proxy, client)
	if err != nil {
		return err
	}

	auth, err := scom.MakeAuth(p.chainID, p.sk)
	if err != nil {
		return err
	}

	tx, err := proxyIns.AddToGroup(auth, gi)
	if err != nil {
		return err
	}

	return scom.CheckTx(p.endPoint, tx, "add to group")
}

func (p *proxyImpl) SetDesc(desc []byte) error {
	client, err := ethclient.DialContext(context.TODO(), p.endPoint)
	if err != nil {
		return err
	}
	defer client.Close()

	proxyIns, err := proxy.NewProxy(p.proxy, client)
	if err != nil {
		return err
	}

	auth, err := scom.MakeAuth(p.chainID, p.sk)
	if err != nil {
		return err
	}

	tx, err := proxyIns.SetDesc(auth, desc)
	if err != nil {
		return err
	}

	return scom.CheckTx(p.endPoint, tx, "set desc")
}

func (p *proxyImpl) Pledge(i uint64, money *big.Int) error {
	client, err := ethclient.DialContext(context.TODO(), p.endPoint)
	if err != nil {
		return err
	}
	defer client.Close()

	proxyIns, err := proxy.NewProxy(p.proxy, client)
	if err != nil {
		return err
	}

	auth, err := scom.MakeAuth(p.chainID, p.sk)
	if err != nil {
		return err
	}

	tx, err := proxyIns.Pledge(auth, i, money)
	if err != nil {
		return err
	}

	return scom.CheckTx(p.endPoint, tx, "pledge")
}

func (p *proxyImpl) PledgeWithdraw(i uint64, ti uint8, money *big.Int) error {
	client, err := ethclient.DialContext(context.TODO(), p.endPoint)
	if err != nil {
		return err
	}
	defer client.Close()

	proxyIns, err := proxy.NewProxy(p.proxy, client)
	if err != nil {
		return err
	}

	auth, err := scom.MakeAuth(p.chainID, p.sk)
	if err != nil {
		return err
	}

	tx, err := proxyIns.Unpledge(auth, i, ti, money)
	if err != nil {
		return err
	}

	return scom.CheckTx(p.endPoint, tx, "unpledge")
}

func (p *proxyImpl) PledgeRewardWithdraw(i uint64, ti uint8, money *big.Int) error {
	client, err := ethclient.DialContext(context.TODO(), p.endPoint)
	if err != nil {
		return err
	}
	defer client.Close()

	proxyIns, err := proxy.NewProxy(p.proxy, client)
	if err != nil {
		return err
	}

	auth, err := scom.MakeAuth(p.chainID, p.sk)
	if err != nil {
		return err
	}

	tx, err := proxyIns.WithdrawPleRwd(auth, i, ti, money)
	if err != nil {
		return err
	}

	return scom.CheckTx(p.endPoint, tx, "pledgeRewardWithdraw")
}

func (p *proxyImpl) AddOrder(oi inter.OrderIn, uSign []byte, pSign []byte) error {
	client, err := ethclient.DialContext(context.TODO(), p.endPoint)
	if err != nil {
		return err
	}
	defer client.Close()

	proxyIns, err := proxy.NewProxy(p.proxy, client)
	if err != nil {
		return err
	}

	auth, err := scom.MakeAuth(p.chainID, p.sk)
	if err != nil {
		return err
	}

	noi := proxy.OrderIn{
		UIndex: oi.UIndex,
		PIndex: oi.PIndex,
		Start:  oi.Start,
		End:    oi.End,
		Size:   oi.Size,
		Nonce:  oi.Nonce,
		TIndex: oi.TIndex,
		Sprice: oi.Sprice,
	}

	tx, err := proxyIns.AddOrder(auth, noi, uSign, pSign)
	if err != nil {
		return err
	}

	return scom.CheckTx(p.endPoint, tx, "add order")
}

func (p *proxyImpl) SubOrder(oi inter.OrderIn, uSign []byte, pSign []byte) error {
	client, err := ethclient.DialContext(context.TODO(), p.endPoint)
	if err != nil {
		return err
	}
	defer client.Close()

	proxyIns, err := proxy.NewProxy(p.proxy, client)
	if err != nil {
		return err
	}

	auth, err := scom.MakeAuth(p.chainID, p.sk)
	if err != nil {
		return err
	}

	noi := proxy.OrderIn{
		UIndex: oi.UIndex,
		PIndex: oi.PIndex,
		Start:  oi.Start,
		End:    oi.End,
		Size:   oi.Size,
		Nonce:  oi.Nonce,
		TIndex: oi.TIndex,
		Sprice: oi.Sprice,
	}

	tx, err := proxyIns.SubOrder(auth, noi, uSign, pSign)
	if err != nil {
		return err
	}

	return scom.CheckTx(p.endPoint, tx, "sub order")
}

func (p *proxyImpl) AddRepair(oi inter.OrderIn, kis []uint64, ksigns [][]byte) error {
	client, err := ethclient.DialContext(context.TODO(), p.endPoint)
	if err != nil {
		return err
	}
	defer client.Close()

	proxyIns, err := proxy.NewProxy(p.proxy, client)
	if err != nil {
		return err
	}

	auth, err := scom.MakeAuth(p.chainID, p.sk)
	if err != nil {
		return err
	}

	noi := proxy.OrderIn{
		UIndex: oi.UIndex,
		PIndex: oi.PIndex,
		Start:  oi.Start,
		End:    oi.End,
		Size:   oi.Size,
		Nonce:  oi.Nonce,
		TIndex: oi.TIndex,
		Sprice: oi.Sprice,
	}

	tx, err := proxyIns.AddRepair(auth, noi, kis, ksigns)
	if err != nil {
		return err
	}

	return scom.CheckTx(p.endPoint, tx, "add repair")
}

func (p *proxyImpl) Recharge(i uint64, ti uint8, isLock bool, money *big.Int) error {
	client, err := ethclient.DialContext(context.TODO(), p.endPoint)
	if err != nil {
		return err
	}
	defer client.Close()

	proxyIns, err := proxy.NewProxy(p.proxy, client)
	if err != nil {
		return err
	}

	auth, err := scom.MakeAuth(p.chainID, p.sk)
	if err != nil {
		return err
	}

	tx, err := proxyIns.Recharge(auth, i, ti, isLock, money)
	if err != nil {
		return err
	}

	return scom.CheckTx(p.endPoint, tx, "recharge")
}

func (p *proxyImpl) Withdraw(i uint64, ti uint8, money *big.Int) error {
	client, err := ethclient.DialContext(context.TODO(), p.endPoint)
	if err != nil {
		return err
	}
	defer client.Close()

	proxyIns, err := proxy.NewProxy(p.proxy, client)
	if err != nil {
		return err
	}

	auth, err := scom.MakeAuth(p.chainID, p.sk)
	if err != nil {
		return err
	}

	tx, err := proxyIns.Withdraw(auth, i, ti, money)
	if err != nil {
		return err
	}

	return scom.CheckTx(p.endPoint, tx, "withdraw")
}

func (p *proxyImpl) ProWithdraw(ps inter.PWIn, kis []uint64, ksigns [][]byte) error {
	client, err := ethclient.DialContext(context.TODO(), p.endPoint)
	if err != nil {
		return err
	}
	defer client.Close()

	sLen := len(kis)
	if sLen > len(ksigns) {
		sLen = len(ksigns)
	}

	type pks struct {
		ki uint64
		s  []byte
	}

	ks := make([]*pks, sLen)

	for i := 0; i < sLen; i++ {
		ks[i] = &pks{
			ki: kis[i],
			s:  ksigns[i],
		}
	}

	sort.Slice(ks, func(i, j int) bool {
		return ks[i].ki < ks[j].ki
	})

	nkis := make([]uint64, sLen)
	nksigns := make([][]byte, sLen)
	for i := 0; i < sLen; i++ {
		nkis[i] = ks[i].ki
		nksigns[i] = ks[i].s
	}

	proxyIns, err := proxy.NewProxy(p.proxy, client)
	if err != nil {
		return err
	}

	auth, err := scom.MakeAuth(p.chainID, p.sk)
	if err != nil {
		return err
	}

	nps := proxy.PWIn{
		PIndex: ps.PIndex,
		TIndex: ps.TIndex,
		Pay:    ps.Pay,
		Lost:   ps.Lost,
	}

	tx, err := proxyIns.ProWithdraw(auth, nps, nkis, nksigns)
	if err != nil {
		return err
	}

	return scom.CheckTx(p.endPoint, tx, "pro withdraw")
}
