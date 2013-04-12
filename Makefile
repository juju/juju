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
	find "$(GOPATH)/src/$(PROJECT)/" -name \*.go | xargs gofmt -w

# Invoke gofmt's "simplify" option to streamline the source code.
simplify:
	find "$(GOPATH)/src/$(PROJECT)/" -name \*.go | xargs gofmt -w -s

# Pattern rule: building a Go file.
%: %.go
	go build -o $@ $<

.PHONY: build check format simplify
