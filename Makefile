SHELL=/usr/bin/env bash

all: build
.PHONY: all

windows: build-windows
.PHONY: windows

unexport GOFLAGS

GOVERSION:=$(shell go version | cut -d' ' -f 3 | sed 's/^go//' | awk -F. '{printf "%d%03d%03d", $$1, $$2, $$3}')
ifeq ($(shell expr $(GOVERSION) \< 1015005), 1)
$(warning Your Golang version is go$(shell expr $(GOVERSION) / 1000000).$(shell expr $(GOVERSION) % 1000000 / 1000).$(shell expr $(GOVERSION) % 1000))
$(error Update Golang to version to at least 1.15.5)
endif

# git modules that need to be loaded
MODULES:=

CLEAN:=
BINS:=

ldflags=-X=github.com/memoio/go-mefs-v2/build.CurrentCommit=+git.$(subst -,.,$(shell git describe --always --match=NeVeRmAtCh --dirty 2>/dev/null || git rev-parse --short HEAD 2>/dev/null))+$(shell date "+%F.%T%Z")
ifneq ($(strip $(LDFLAGS)),)
	ldflags+=-extldflags=$(LDFLAGS)
endif

GOFLAGS+=-ldflags="-s -w $(ldflags)"

mefs: $(BUILD_DEPS)
	rm -f mefs
	go build $(GOFLAGS) -o mefs ./app/mefs

.PHONY: mefs
BINS+=mefs

keeper: $(BUILD_DEPS)
	rm -f mefs-keeper
	go build $(GOFLAGS) -o mefs-keeper ./app/keeper

.PHONY: mefs-keeper
BINS+=mefs-keeper 

user: $(BUILD_DEPS)
	rm -f mefs-user
	go build $(GOFLAGS) -o mefs-user ./app/user

.PHONY: mefs-user
BINS+=mefs-user

provider: $(BUILD_DEPS)
	rm -f mefs-provider
	go build $(GOFLAGS) -o mefs-provider ./app/provider

.PHONY: mefs-provider
BINS+=mefs-provider

build: mefs keeper user provider
.PHONY: build

mefs-windows: $(BUILD_DEPS)
	rm -f mefs.exe
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(GOFLAGS) -o mefs.exe ./app/mefs

.PHONY: mefs-windows
BINS+=mefs.exe

keeper-windows: $(BUILD_DEPS)
	rm -f mefs-keeper.exe
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(GOFLAGS) -o mefs-keeper.exe ./app/keeper

.PHONY: mefs-keeper-windows
BINS+=mefs-keeper.exe 

user-windows: $(BUILD_DEPS)
	rm -f mefs-user.exe
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(GOFLAGS) -o mefs-user.exe ./app/user

.PHONY: mefs-user-windows
BINS+=mefs-user.exe

provider-windows: $(BUILD_DEPS)
	rm -f mefs-provider.exe
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(GOFLAGS) -o mefs-provider.exe ./app/provider

.PHONY: mefs-provider-windows
BINS+=mefs-provider.exe

build-windows: mefs-windows keeper-windows user-windows provider-windows
.PHONY: build-windows


clean:
	rm -rf $(BINS)
.PHONY: clean