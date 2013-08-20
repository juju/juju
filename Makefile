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
  golang-go
  mercurial
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
	go fmt ./...

# Remove test executables.
clean:
	find . -name '*.test' -print0 | xargs -r0 $(RM) -v

# Install packages required to develop Juju and run tests.
install-dependencies:
	sudo apt-get install $(strip $(DEPENDENCIES))
	@echo
	@echo "Make sure you have MongoDB installed.  See the README file."

# Invoke gofmt's "simplify" option to streamline the source code.
simplify:
	gofmt -w -s .


.PHONY: build check format clean install-dependencies simplify
