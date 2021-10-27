package client

import (
	"context"
	"io/ioutil"
	"net/http"
	"path"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/memoio/go-mefs-v2/app/api"
	"github.com/memoio/go-mefs-v2/lib/utils/paths"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

func GetMemoClientInfo(repoDir string) (string, http.Header, error) {
	repoPath, err := paths.GetRepoPath(repoDir)
	if err != nil {
		return "", nil, err
	}

	//tokePath := path.Join(repoPath, "token")
	rpcPath := path.Join(repoPath, "api")

	//tokenBytes, err := ioutil.ReadFile(tokePath)
	// if err != nil {
	// 	return "", nil, err
	// }
	rpcBytes, err := ioutil.ReadFile(rpcPath)
	if err != nil {
		return "", nil, err
	}

	headers := http.Header{}
	//headers.Add("Authorization", "Bearer "+string(tokenBytes))
	apima, err := multiaddr.NewMultiaddr(string(rpcBytes))
	if err != nil {
		return "", nil, err
	}

	_, addr, err := manet.DialArgs(apima)
	if err != nil {
		return "", nil, err
	}

	addr = "ws://" + addr + "/rpc/v0"
	return addr, headers, nil
}

func NewGenericNode(ctx context.Context, addr string, requestHeader http.Header) (api.FullNode, jsonrpc.ClientCloser, error) {
	var res api.FullNodeStruct
	closer, err := jsonrpc.NewMergeClient(ctx, addr, "Memoriae",
		api.GetInternalStructs(&res), requestHeader)

	return &res, closer, err
}
