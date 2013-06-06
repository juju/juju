# Makefile for juju-core.
PROJECT=launchpad.net/juju-core

# Default target.  Compile, just to see if it will.
build:
	go build $(PROJECT)/...

# Run tests.
check:
	go test $(PROJECT)/...

# Reformat the source files.
format:
	go fmt $(PROJECT)/...

# Install packages required to develop Juju and run tests.
install-dependencies:
	sudo apt-get install build-essential bzr zip git-core mercurial distro-info-data golang-go mongodb

# Invoke gofmt's "simplify" option to streamline the source code.
simplify:
	find "$(GOPATH)/src/$(PROJECT)/" -name \*.go | xargs gofmt -w -s


.PHONY: build check format install-dependencies simplify
