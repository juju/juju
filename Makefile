#
# Makefile for juju-core.
#

PROJECT := launchpad.net/juju-core
PROJECT_DIR := $(shell go list -e -f '{{.Dir}}' $(PACKAGE))

ifndef GOPATH
$(warning You need to set up a GOPATH.  See the README file.)
endif

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

else # --------------------------------

build:
	$(error Cannot build package outside of GOPATH)

check:
	$(error Cannot check package outside of GOPATH)

endif
# End of GOPATH-dependent targets.

# Reformat the source files.
format:
	gofmt -w -l .

# Invoke gofmt's "simplify" option to streamline the source code.
simplify:
	gofmt -w -l -s .

# Clean the tree, including removing test executables.
clean:
	find . -name '*.test' -print0 | xargs -r0 $(RM) -v

# Install juju into $GOPATH/bin.
install:
	go install -v $(PROJECT)/...

# Install packages required to develop Juju and run tests. The stable
# PPA includes the required mongodb-server binaries.
install-dependencies:
	@echo Adding juju PPAs for golang and mongodb-server
	@sudo apt-add-repository ppa:juju/golang
	@sudo apt-add-repository ppa:juju/stable
	@sudo apt-get update
	@echo Installing dependencies
	@sudo apt-get install $(strip $(DEPENDENCIES))


.PHONY: build check format simplify clean install install-dependencies
