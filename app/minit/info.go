package minit

import (
	"log"
	"runtime"
	"sort"

	"github.com/libp2p/go-libp2p-core/host"

	"github.com/memoio/go-mefs-v2/build"
)

type Node interface {
	Start() error
	RunDaemon(chan interface{}) error
	Online() bool
	GetHost() host.Host
}

func PrintVersion() {
	v := build.UserVersion()
	log.Printf("Memoriae version: %s", v)
	log.Printf("System version: %s", runtime.GOARCH+"/"+runtime.GOOS)
	log.Printf("Golang version: %s", runtime.Version())
}

// printSwarmAddrs prints the addresses of the host
func PrintSwarmAddrs(node Node) {
	if !node.Online() {
		log.Println("Swarm not listening, running in offline mode.")
		return
	}

	var lisAddrs []string
	ifaceAddrs, err := node.GetHost().Network().InterfaceListenAddresses()
	if err != nil {
		log.Fatalf("failed to read listening addresses: %s", err)
	}
	for _, addr := range ifaceAddrs {
		lisAddrs = append(lisAddrs, addr.String())
	}
	sort.Strings(lisAddrs)
	for _, addr := range lisAddrs {
		log.Printf("Swarm listening on %s\n", addr)
	}
}
