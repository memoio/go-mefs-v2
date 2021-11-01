SHELL=/usr/bin/env bash

all: build
.PHONY: all

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

GOFLAGS+=-ldflags="$(ldflags)"

mefs: $(BUILD_DEPS)
	rm -f mefs
	go build $(GOFLAGS) -o mefs ./app/mefs

.PHONY: mefs
BINS+=mefs


build: mefs

.PHONY: build

clean:
	rm -rf $(BINS)
.PHONY: clean