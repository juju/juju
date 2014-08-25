#
# Makefile for juju-core.
#

ifndef GOPATH
$(warning You need to set up a GOPATH.  See the README file.)
endif

PROJECT := github.com/juju/juju
PROJECT_DIR := $(shell go list -e -f '{{.Dir}}' $(PROJECT))

ifeq ($(shell uname -p | sed -r 's/.*(86|armel|armhf).*/golang/'), golang)
	GO_C := golang
	INSTALL_FLAGS := 
else
	GO_C := gccgo-4.9  gccgo-go
	INSTALL_FLAGS := -gccgoflags=-static-libgo
endif

define DEPENDENCIES
  ca-certificates
  bzr
  distro-info-data
  git-core
  mercurial
  juju-local
  zip
  $(GO_C)
endef

default: build

# Start of GOPATH-dependent targets. Some targets only make sense -
# and will only work - when this tree is found on the GOPATH.
ifeq ($(CURDIR),$(PROJECT_DIR))

ifeq ($(JUJU_MAKE_GODEPS),true)
$(GOPATH)/bin/godeps:
	go get launchpad.net/godeps

godeps: $(GOPATH)/bin/godeps
	$(GOPATH)/bin/godeps -u dependencies.tsv
else
godeps:
	@echo "skipping godeps"
endif

build: godeps
	go build $(PROJECT)/...

check: godeps
	go test -test.timeout=1200s $(PROJECT)/...

install: godeps
	go install $(INSTALL_FLAGS) -v $(PROJECT)/...

clean:
	go clean $(PROJECT)/...

else # --------------------------------

build:
	$(error Cannot $@; $(CURDIR) is not on GOPATH)

check:
	$(error Cannot $@; $(CURDIR) is not on GOPATH)

install:
	$(error Cannot $@; $(CURDIR) is not on GOPATH)

clean:
	$(error Cannot $@; $(CURDIR) is not on GOPATH)

endif
# End of GOPATH-dependent targets.

# Reformat source files.
format:
	gofmt -w -l .

# Reformat and simplify source files.
simplify:
	gofmt -w -l -s .

# Install packages required to develop Juju and run tests. The stable
# PPA includes the required mongodb-server binaries.
install-dependencies:
ifeq ($(shell lsb_release -cs|sed -r 's/precise/old/'),old)
	@echo Adding juju PPAs for golang and mongodb-server
	@sudo apt-add-repository --yes ppa:juju/golang
	@sudo apt-add-repository --yes ppa:juju/stable
	@sudo apt-get update
endif
	@echo Installing dependencies
	@sudo apt-get --yes install --no-install-recommends \
	$(strip $(DEPENDENCIES)) \
	$(shell apt-cache madison juju-mongodb mongodb-server | head -1 | cut -d '|' -f1)

# Install bash_completion
install-etc:
	@echo Installing bash completion
	@sudo install -o root -g root -m 644 etc/bash_completion.d/juju-core /etc/bash_completion.d

.PHONY: build check install
.PHONY: clean format simplify
.PHONY: install-dependencies
