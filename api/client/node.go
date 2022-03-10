package client

import (
	"context"
	"io/ioutil"
	"net/http"
	"path"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/api/httpio"
	"github.com/memoio/go-mefs-v2/lib/utils"
)

func GetMemoClientInfo(repoDir string) (string, http.Header, error) {
	repoPath, err := utils.GetRepoPath(repoDir)
	if err != nil {
		return "", nil, err
	}

	tokePath := path.Join(repoPath, "token")
	tokenBytes, err := ioutil.ReadFile(tokePath)
	if err != nil {
		return "", nil, err
	}

	rpcPath := path.Join(repoPath, "api")
	rpcBytes, err := ioutil.ReadFile(rpcPath)
	if err != nil {
		return "", nil, err
	}

	headers := http.Header{}
	headers.Add("Authorization", "Bearer "+string(tokenBytes))
	apima, err := multiaddr.NewMultiaddr(string(rpcBytes))
	if err != nil {
		return "", nil, err
	}

	_, addr, err := manet.DialArgs(apima)
	if err != nil {
		return "", nil, err
	}

	//addr = "http://" + addr + "/rpc/v0"
	return addr, headers, nil
}

func NewGenericNode(ctx context.Context, addr string, requestHeader http.Header) (api.FullNode, jsonrpc.ClientCloser, error) {
	var res api.FullNodeStruct
	closer, err := jsonrpc.NewMergeClient(ctx, "ws://"+addr+"/rpc/v0", "Memoriae",
		api.GetInternalStructs(&res), requestHeader)

	return &res, closer, err
}

// create an user node with package api
func NewUserNode(ctx context.Context, addr string, requestHeader http.Header) (api.UserNode, jsonrpc.ClientCloser, error) {
	var res api.UserNodeStruct
	re := httpio.ReaderParamEncoder("http://" + addr + "/rpc/streams/v0/push")
	closer, err := jsonrpc.NewMergeClient(ctx, "ws://"+addr+"/rpc/v0", "Memoriae",
		api.GetInternalStructs(&res), requestHeader, re)

	return &res, closer, err
}

func NewProviderNode(ctx context.Context, addr string, requestHeader http.Header) (api.ProviderNode, jsonrpc.ClientCloser, error) {
	var res api.ProviderNodeStruct
	closer, err := jsonrpc.NewMergeClient(ctx, "ws://"+addr+"/rpc/v0", "Memoriae",
		api.GetInternalStructs(&res), requestHeader)

	return &res, closer, err
}
