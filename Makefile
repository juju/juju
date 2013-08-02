#
# Makefile for juju-core.
#

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


# Default target.  Compile, just to see if it will.
build:
	go build ./...

# Run tests.
check:
	go test ./...

# Reformat the source files.
format:
	go fmt ./...

# Install packages required to develop Juju and run tests.
install-dependencies:
	sudo apt-get install $(strip $(DEPENDENCIES))
	@echo
	@echo "Make sure you have MongoDB installed.  See the README file."

# Invoke gofmt's "simplify" option to streamline the source code.
simplify:
	gofmt -w -s .


.PHONY: build check format install-dependencies simplify
