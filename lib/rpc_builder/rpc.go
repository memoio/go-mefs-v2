package rpc_builder

import (
	"reflect"

	"github.com/filecoin-project/go-jsonrpc"
	xerrors "github.com/pkg/errors"
)

// rpc build based on go-jsonrpc

const defaultMethodName = "API"

type RPCService interface {
}

type RPCBuilder struct {
	namespace []string
	apiStruct []interface{}
}

func NewBuiler() *RPCBuilder {
	return &RPCBuilder{}
}

func (builder *RPCBuilder) NameSpace(nameSpaece string) *RPCBuilder {
	builder.namespace = append(builder.namespace, nameSpaece)
	return builder
}

func (builder *RPCBuilder) AddServices(services ...RPCService) error {
	for _, service := range services {
		err := builder.AddService(service)
		if err != nil {
			return err
		}
	}
	return nil
}

func (builder *RPCBuilder) AddService(service RPCService) error {
	serviceV := reflect.ValueOf(service)
	apiMethod := serviceV.MethodByName(defaultMethodName)
	if !apiMethod.IsValid() {
		return xerrors.New("expect API function")
	}

	apiImpls := apiMethod.Call([]reflect.Value{})

	for _, apiImpl := range apiImpls {
		rt := reflect.TypeOf(apiImpl)
		rv := reflect.ValueOf(apiImpl)
		if rt.Kind() == reflect.Array {
			apiLen := rv.Len()
			for i := 0; i < apiLen; i++ {
				ele := rv.Index(i)
				if ele.IsValid() {
					builder.apiStruct = append(builder.apiStruct, apiImpl.Interface())
				}
			}
		} else {
			builder.apiStruct = append(builder.apiStruct, apiImpl.Interface())
		}
	}

	return nil
}

func (builder *RPCBuilder) Build() *jsonrpc.RPCServer {
	server := jsonrpc.NewServer()

	for _, nameSpace := range builder.namespace {
		for _, apiStruct := range builder.apiStruct {
			server.Register(nameSpace, apiStruct)
		}
	}
	return server
}
