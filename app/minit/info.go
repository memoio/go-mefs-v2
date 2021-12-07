package minit

import (
	"context"
	"log"
	"runtime"

	"github.com/memoio/go-mefs-v2/build"
)

type Node interface {
	Start() error
	Stop(context.Context) error
	RunDaemon() error
	Online() bool
}

func PrintVersion() {
	v := build.UserVersion()
	log.Printf("Memoriae version: %s", v)
	log.Printf("System version: %s", runtime.GOARCH+"/"+runtime.GOOS)
	log.Printf("Golang version: %s", runtime.Version())
}
