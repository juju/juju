# Makefile for the bundlechanges library.

PROJECT := github.com/juju/bundlechanges
PROJECT_DIR := $(shell go list -e -f '{{.Dir}}' $(PROJECT))

ifeq ($(shell uname -p | sed -r 's/.*(x86|armel|armhf).*/golang/'), golang)
	GO_C := golang
	INSTALL_FLAGS :=
else
	GO_C := gccgo-4.9 gccgo-go
	INSTALL_FLAGS := -gccgoflags=-static-libgo
endif

define DEPENDENCIES
  build-essential
  bzr
  git
  $(GO_C)
endef

default: build

$(GOPATH)/bin/godeps:
	go get -v github.com/rogpeppe/godeps

# Start of GOPATH-dependent targets. Some targets only make sense -
# and will only work - when this tree is found on the GOPATH.
ifeq ($(CURDIR),$(PROJECT_DIR))

build:
	go build $(PROJECT)/...

check:
	go test $(PROJECT)/...

install:
	go install $(INSTALL_FLAGS) -v $(PROJECT)/...

clean:
	go clean $(PROJECT)/...

else

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

# Update the project Go dependencies to the required revision.
deps: $(GOPATH)/bin/godeps
	$(GOPATH)/bin/godeps -u dependencies.tsv

# Generate the dependencies file.
create-deps: $(GOPATH)/bin/godeps
	godeps -t $(shell go list $(PROJECT)/...) > dependencies.tsv || true

# Install packages required to develop the bundlechanges lib and run tests.
APT_BASED := $(shell command -v apt-get >/dev/null; echo $$?)
sysdeps:
ifeq ($(APT_BASED),0)
ifeq ($(shell lsb_release -cs|sed -r 's/precise|quantal|raring/old/'),old)
	@echo Adding PPAs for golang and mongodb
	@sudo apt-add-repository --yes ppa:juju/golang
	@sudo apt-add-repository --yes ppa:juju/stable
endif
	@echo Installing dependencies
	sudo apt-get update
	@sudo apt-get --yes install $(strip $(DEPENDENCIES)) \
	$(shell apt-cache madison juju-mongodb mongodb-server | head -1 | cut -d '|' -f1)
else
	@echo sysdeps runs only on systems with apt-get
	@echo on OS X with homebrew try: brew install bazaar mongodb
endif

help:
	@echo -e 'Juju Bundle Changes - list of make targets:\n'
	@echo 'make - Build the package.'
	@echo 'make check - Run tests.'
	@echo 'make install - Install the package.'
	@echo 'make clean - Remove object files from package source directories.'
	@echo 'make sysdeps - Install the development environment system packages.'
	@echo 'make deps - Set up the project Go dependencies.'
	@echo 'make create-deps - Generate the Go dependencies file.'
	@echo 'make format - Format the source files.'

.PHONY: build check clean format help install sysdeps
