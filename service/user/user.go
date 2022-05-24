package user

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/gorilla/mux"
	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/crypto/pdp"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/service/data"
	"github.com/memoio/go-mefs-v2/service/user/lfs"
	uorder "github.com/memoio/go-mefs-v2/service/user/order"
	"github.com/memoio/go-mefs-v2/submodule/connect/readpay"
	"github.com/memoio/go-mefs-v2/submodule/metrics"
	"github.com/memoio/go-mefs-v2/submodule/node"
)

var logger = logging.Logger("user")

var _ api.UserNode = (*UserNode)(nil)

type UserNode struct {
	sync.RWMutex

	*node.BaseNode

	*lfs.LfsService

	api.IDataService

	ctx context.Context

	ready bool
}

func New(ctx context.Context, opts ...node.BuilderOpt) (*UserNode, error) {
	bn, err := node.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	ds := bn.MetaStore()

	segStore, err := segment.NewSegStore(bn.Repo.FileStore())
	if err != nil {
		return nil, err
	}

	pri, err := bn.RoleMgr.RoleSelf(ctx)
	if err != nil {
		return nil, err
	}

	localAddr, err := address.NewAddress(pri.ChainVerifyKey)
	if err != nil {
		return nil, err
	}

	ki, err := bn.WalletExport(ctx, localAddr, bn.PassWord())
	if err != nil {
		return nil, err
	}

	privBytes := ki.SecretKey

	privBytes = append(privBytes, byte(types.PDP))

	keyset, err := pdp.GenerateKeyWithSeed(pdpcommon.PDPV2, privBytes)
	if err != nil {
		return nil, err
	}

	sp := readpay.NewSender(localAddr, bn.LocalWallet, ds)

	ids := data.New(ds, segStore, bn.NetServiceImpl, bn.RoleMgr, sp)

	oc := bn.Repo.Config().Order

	om := uorder.NewOrderMgr(ctx, bn.RoleID(), keyset.VerifyKey().Hash(), oc.Price, oc.Duration*86400, oc.Wait, ds, bn.PushPool, bn.RoleMgr, ids, bn.NetServiceImpl, bn.ContractMgr)

	ls, err := lfs.New(ctx, bn.RoleID(), keyset, ds, segStore, om)
	if err != nil {
		return nil, err
	}

	un := &UserNode{
		BaseNode:     bn,
		LfsService:   ls,
		IDataService: ids,
		ctx:          ctx,
	}

	un.RegisterAddSeqFunc(om.AddOrderSeq)
	un.RegisterDelSegFunc(om.RemoveSeg)

	return un, nil
}

// start service related
func (u *UserNode) Start(perm bool) error {
	u.Perm = perm
	if u.Repo.Config().Net.Name == "test" {
		go u.OpenTest()
	} else {
		u.RoleMgr.Start()
	}

	// register net msg handle
	u.GenericService.Register(pb.NetMessage_SayHello, u.DefaultHandler)
	u.GenericService.Register(pb.NetMessage_Get, u.HandleGet)

	u.TxMsgHandle.Register(u.BaseNode.TxMsgHandler)
	u.BlockHandle.Register(u.BaseNode.TxBlockHandler)

	u.HttpHandle.PathPrefix("/gateway").HandlerFunc(u.ServeRemote(u.Perm))
	u.HttpHandle.Handle("/debug/metrics", metrics.NewExporter())
	u.HttpHandle.PathPrefix("/").Handler(http.DefaultServeMux)

	if u.Perm {
		u.RPCServer.Register("Memoriae", api.PermissionedUserAPI(metrics.MetricedUserAPI(u)))
	} else {
		u.RPCServer.Register("Memoriae", metrics.MetricedUserAPI(u))
	}

	go func() {
		// wait for sync
		u.PushPool.Start()
		for {
			if u.PushPool.Ready() {
				break
			} else {
				logger.Debug("wait for sync")
				time.Sleep(5 * time.Second)
			}
		}

		// wait for register
		err := u.Register()
		if err != nil {
			return
		}

		u.ready = true

		// start lfs service and its ordermgr service
		u.LfsService.Start()
	}()

	logger.Info("Start user: ", u.RoleID())
	return nil
}

func (u *UserNode) Shutdown(ctx context.Context) error {
	u.LfsService.Stop()
	return u.BaseNode.Shutdown(ctx)
}

func (u *UserNode) Ready(ctx context.Context) bool {
	return u.ready
}

func (u *UserNode) ServeRemote(perm bool) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if perm {
			if !auth.HasPerm(r.Context(), nil, api.PermAdmin) {
				w.WriteHeader(401)
				_ = json.NewEncoder(w).Encode(struct{ Error string }{"unauthorized: missing write permission"})
				return
			}
		}
		u.ServeRemoteHTTP(w, r)
	}
}

func (u *UserNode) ServeRemoteHTTP(w http.ResponseWriter, r *http.Request) {
	mux := mux.NewRouter()

	mux.HandleFunc("/gateway/state", u.LfsService.GetState).Methods("GET")
	mux.HandleFunc("/gateway/cid/{cid}", u.LfsService.GetFileByCID).Methods("GET")
	mux.HandleFunc("/gateway/cid/{cid}/{st}/{le}", u.LfsService.GetFileByCID).Methods("GET")
	mux.HandleFunc("/gateway/{bn}/{on}", u.LfsService.GetFile).Methods("GET")
	mux.HandleFunc("/gateway/{bn}/{on}/{st}/{le}", u.LfsService.GetFile).Methods("GET")

	mux.ServeHTTP(w, r)
}
