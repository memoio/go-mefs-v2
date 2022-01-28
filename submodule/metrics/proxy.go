package metrics

import (
	"context"
	"reflect"

	"github.com/memoio/go-mefs-v2/api"
	"go.opencensus.io/tag"
)

// for measure api latency
func MetricedFullAPI(a api.FullNode) api.FullNode {
	var out api.FullNodeStruct
	proxy(a, &out.Internal)
	proxy(a, &out.CommonStruct.Internal)
	return &out
}

func MetricedUserAPI(a api.UserNode) api.UserNode {
	var out api.UserNodeStruct
	proxy(a, &out.Internal)
	proxy(a, &out.CommonStruct.Internal)
	return &out
}

func MetricedProviderAPI(a api.ProviderNode) api.ProviderNode {
	var out api.ProviderNodeStruct
	proxy(a, &out.Internal)
	proxy(a, &out.CommonStruct.Internal)
	return &out
}

func MetricedKeeperAPI(a api.KeeperNode) api.KeeperNode {
	var out api.KeeperNodeStruct
	proxy(a, &out.Internal)
	proxy(a, &out.CommonStruct.Internal)
	return &out
}

func proxy(in interface{}, out interface{}) {
	rint := reflect.ValueOf(out).Elem()
	ra := reflect.ValueOf(in)

	for f := 0; f < rint.NumField(); f++ {
		field := rint.Type().Field(f)
		fn := ra.MethodByName(field.Name)

		rint.Field(f).Set(reflect.MakeFunc(field.Type, func(args []reflect.Value) (results []reflect.Value) {
			ctx := args[0].Interface().(context.Context)
			// upsert function name into context
			ctx, _ = tag.New(ctx, tag.Upsert(APIMethod, field.Name))
			stop := Timer(ctx, APIRequestDuration)
			defer stop()
			// pass tagged ctx back into function call
			args[0] = reflect.ValueOf(ctx)
			return fn.Call(args)
		}))

	}
}
