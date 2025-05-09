module github.com/memoio/go-mefs-v2

go 1.15

require (
	contrib.go.opencensus.io/exporter/prometheus v0.4.1
	github.com/OneOfOne/xxhash v1.2.5 // indirect
	github.com/bits-and-blooms/bitset v1.2.1
	github.com/btcsuite/btcd v0.22.1
	github.com/dgraph-io/badger/v2 v2.2007.4
	github.com/ethereum/go-ethereum v1.10.16
	github.com/filecoin-project/go-jsonrpc v0.1.5
	github.com/fxamacker/cbor/v2 v2.3.0
	github.com/gbrlsnchs/jwt/v3 v3.0.1
	github.com/gogo/protobuf v1.3.2
	github.com/golang/protobuf v1.5.2
	github.com/google/uuid v1.3.0
	github.com/gorilla/mux v1.8.0
	github.com/hashicorp/golang-lru v0.5.5-0.20210104140557-80c98217689d
	github.com/herumi/bls-eth-go-binary v0.0.0-20210917013441-d37c07cfda4e
	github.com/howeyc/gopass v0.0.0-20210920133722-c8aef6fb66ef
	github.com/ipfs/go-cid v0.2.0
	github.com/ipfs/go-datastore v0.5.1
	github.com/ipfs/go-ds-leveldb v0.5.0
	github.com/ipfs/go-fs-lock v0.0.7
	github.com/ipfs/go-ipld-format v0.2.0
	github.com/ipfs/go-unixfs v0.2.6
	github.com/ipld/go-codec-dagpb v1.3.0
	github.com/ipld/go-ipld-prime v0.16.0
	github.com/jbenet/goprocess v0.1.4
	github.com/kilic/bls12-381 v0.1.0
	github.com/klauspost/compress v1.14.4
	github.com/klauspost/reedsolomon v1.9.15
	github.com/libp2p/go-libp2p v0.18.0
	github.com/libp2p/go-libp2p-core v0.15.1
	github.com/libp2p/go-libp2p-discovery v0.6.0
	github.com/libp2p/go-libp2p-kad-dht v0.16.0
	github.com/libp2p/go-libp2p-pubsub v0.6.1
	github.com/libp2p/go-libp2p-resource-manager v0.1.5
	github.com/libp2p/go-libp2p-swarm v0.10.2
	github.com/libp2p/go-msgio v0.2.0
	github.com/memoio/contractsv2 v0.0.0-00010101000000-000000000000
	github.com/memoio/minio v0.2.6
	github.com/memoio/relay v0.0.0-00010101000000-000000000000
	github.com/memoio/smt v0.2.1
	github.com/mgutz/ansi v0.0.0-20170206155736-9520e82c474b
	github.com/minio/blake2b-simd v0.0.0-20160723061019-3f5f724cb5b1
	github.com/minio/cli v1.22.0
	github.com/minio/madmin-go v1.3.6
	github.com/minio/minio-go/v7 v7.0.23
	github.com/minio/pkg v1.1.20
	github.com/mitchellh/go-homedir v1.1.0
	github.com/modood/table v0.0.0-20200225102042-88de94bb9876
	github.com/mr-tron/base58 v1.2.0
	github.com/multiformats/go-multiaddr v0.5.0
	github.com/multiformats/go-multiaddr-dns v0.3.1
	github.com/multiformats/go-multihash v0.1.0
	github.com/prometheus/client_golang v1.12.1
	github.com/sakeven/RbTree v0.0.0-20190505104653-18ee3093df2f
	github.com/schollz/progressbar/v3 v3.8.6
	github.com/shirou/gopsutil/v3 v3.22.2
	github.com/stretchr/testify v1.7.1
	github.com/syndtr/goleveldb v1.0.1-0.20210819022825-2ae1ddf74ef7
	github.com/urfave/cli/v2 v2.6.0
	github.com/zeebo/blake3 v0.2.3
	go.opencensus.io v0.23.0
	go.uber.org/zap v1.21.0
	golang.org/x/crypto v0.0.0-20220518034528-6f7dac969898
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
	lukechampine.com/frand v1.4.2
	memoc v0.0.0-00010101000000-000000000000
)

replace (
	github.com/memoio/contractsv2 => ../memov2-contractsv2
	github.com/memoio/relay => ../relay
	memoc => ../memo-go-contracts-v2
)
