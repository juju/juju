#
# Makefile for juju-core.
#

ifndef GOPATH
$(warning You need to set up a GOPATH.  See the README file.)
endif

PROJECT := launchpad.net/juju-core
PROJECT_DIR := $(shell go list -e -f '{{.Dir}}' $(PACKAGE))

define DEPENDENCIES
  build-essential
  bzr
  distro-info-data
  git-core
  golang
  mercurial
  mongodb-server
  zip
endef

default: build

# Start of GOPATH-dependent targets. Some targets only make sense -
# and will only work - when this tree is found on the GOPATH.
ifeq ($(CURDIR),$(PROJECT_DIR))

build:
	go build $(PROJECT)/...

check:
	go test $(PROJECT)/...

install:
	go install -v $(PROJECT)/...

clean:
	go clean $(PROJECT)/...

else # --------------------------------

build:
	$(error Cannot build package outside of GOPATH)

check:
	$(error Cannot check package outside of GOPATH)

install:
	$(error Cannot install package from outside of GOPATH)

clean:
	$(error Cannot clean package from outside of GOPATH)

endif
# End of GOPATH-dependent targets.

# Reformat the source files.
format:
	gofmt -w -l .

# Invoke gofmt's "simplify" option to streamline the source code.
simplify:
	gofmt -w -l -s .

# Install packages required to develop Juju and run tests. The stable
# PPA includes the required mongodb-server binaries. However, neither
# PPA works on Saucy just yet.
install-dependencies:
ifneq ($(shell lsb_release -cs),saucy)
	@echo Adding juju PPAs for golang and mongodb-server
	@sudo apt-add-repository --yes ppa:juju/golang
	@sudo apt-add-repository --yes ppa:juju/stable
	@sudo apt-get update
endif
	@echo Installing dependencies
	@sudo apt-get --yes install $(strip $(DEPENDENCIES))


.PHONY: build check install
.PHONY: clean format simplify
.PHONY: install-dependencies
