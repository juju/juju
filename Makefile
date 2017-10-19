#
# Makefile for juju-core.
#

ifndef GOPATH
$(warning You need to set up a GOPATH.  See the README file.)
endif

PROJECT := github.com/juju/juju
PROJECT_DIR := $(shell go list -e -f '{{.Dir}}' $(PROJECT))

# Allow the tests to take longer on arm platforms.
ifeq ($(shell uname -p | sed -r 's/.*(armel|armhf|aarch64).*/golang/'), golang)
	TEST_TIMEOUT := 2400s
else
	TEST_TIMEOUT := 1500s
endif

GO_C = golang-1.9
INSTALL_FLAGS =

# Enable verbose testing for reporting.
ifeq ($(VERBOSE_CHECK), 1)
	CHECK_ARGS = -v
else
	CHECK_ARGS =
endif

define DEPENDENCIES
  ca-certificates
  bzip2
  distro-info-data
  git
  zip
endef

default: build

# Start of GOPATH-dependent targets. Some targets only make sense -
# and will only work - when this tree is found on the GOPATH.
ifeq ($(CURDIR),$(PROJECT_DIR))

ifeq ($(JUJU_MAKE_GODEPS),true)
$(GOPATH)/bin/godeps:
	go get github.com/rogpeppe/godeps

godeps: $(GOPATH)/bin/godeps
	$(GOPATH)/bin/godeps -u dependencies.tsv
else
godeps:
	@echo "skipping godeps"
endif

build: godeps
	go build $(PROJECT)/...

check: godeps
	go test $(CHECK_ARGS) -test.timeout=$(TEST_TIMEOUT) $(PROJECT)/...

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

rebuild-dependencies.tsv: godeps
	# godeps invoked this way includes 'github.com/juju/juju' as part of
	# the content, which we want to filter out.
	# '-t' is not needed on newer versions of godeps, but is still supported.
	godeps -t ./... | grep -v "^github.com/juju/juju\s" > dependencies.tsv

# Install packages required to develop Juju and run tests. The stable
# PPA includes the required mongodb-server binaries.
install-dependencies:
	@echo Adding juju PPA for mongodb
	@sudo apt-add-repository --yes ppa:juju/stable
	@sudo apt-get update
	@echo Installing dependencies
	@sudo apt-get --yes install  \
	$(strip $(DEPENDENCIES)) \
	$(shell apt-cache madison juju-mongodb3.2 juju-mongodb mongodb-server | head -1 | cut -d '|' -f1)

# Install bash_completion
install-etc:
	@echo Installing bash completion
	@sudo install -o root -g root -m 644 etc/bash_completion.d/juju /usr/share/bash-completion/completions
	@sudo install -o root -g root -m 644 etc/bash_completion.d/juju-version /usr/share/bash-completion/completions

setup-lxd:
ifeq ($(shell ifconfig lxdbr0 2>&1 | grep -q "inet addr" && echo true),true)
	@echo IPv4 networking is already setup for LXD.
	@echo run "sudo scripts/setup-lxd.sh" to reconfigure IPv4 networking
else
	@echo Setting up IPv4 networking for LXD
	@sudo scripts/setup-lxd.sh || true
endif


GOCHECK_COUNT="$(shell go list -f '{{join .Deps "\n"}}' github.com/juju/juju/... | grep -c "gopkg.in/check.v*")"
check-deps:
	@echo "$(GOCHECK_COUNT) instances of gocheck not in test code"

install-go:
	@echo Installing go-1.9 snap
	@sudo snap install go --channel=1.9/stable --classic

.PHONY: build check install
.PHONY: clean format simplify
.PHONY: install-dependencies
.PHONY: rebuild-dependencies.tsv
.PHONY: check-deps
.PHONY: install-go
