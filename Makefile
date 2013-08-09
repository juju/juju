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

# Install juju into $GOPATH/bin.
install:
	go install -v $(PROJECT)/...

# Install packages required to develop Juju and run tests.
install-dependencies:
	sudo apt-get install build-essential bzr zip git-core mercurial distro-info-data golang-go
	@echo
	@echo "Make sure you have MongoDB installed.  See the README file."
	@if [ -z "$(GOPATH)" ]; then \
		echo; \
		echo "You need to set up a GOPATH.  See the README file."; \
	fi

# Invoke gofmt's "simplify" option to streamline the source code.
simplify:
	find "$(GOPATH)/src/$(PROJECT)/" -name \*.go | xargs gofmt -w -s


.PHONY: build check format install-dependencies simplify
