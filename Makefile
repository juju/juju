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

ifneq ($(shell uname -p | sed -r 's/.*(86|armel|armhf|aarch64|ppc64le|s390x).*/golang/'), golang)
	$(error Unsupported CPU architecture.) 
endif

# Enable verbose testing for reporting.
ifeq ($(VERBOSE_CHECK), 1)
	CHECK_ARGS = -v
else
	CHECK_ARGS =
endif

define DEPENDENCIES
  ca-certificates
  bzip2
  bzr
  distro-info-data
  git
  juju-local
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

build: godeps go-build

add-patches:
	cat $(PWD)/patches/*.diff | patch -f -u -p1 -r- -d $(PWD)/../../../

#this is useful to run after release-build, or as needed
remove-patches:
	cat $(PWD)/patches/*.diff | patch -f -R -u -p1 -r- -d $(PWD)/../../../

release-build: godeps add-patches go-build

release-install: godeps add-patches go-install remove-patches

pre-check:
	@echo running pre-test checks
	@$(PROJECT_DIR)/scripts/verify.bash

check: godeps pre-check
	go test $(CHECK_ARGS) -test.timeout=$(TEST_TIMEOUT) $(PROJECT)/...

install: godeps go-install

clean:
	go clean $(PROJECT)/...

go-install:
	go install -v $(PROJECT)/...

go-build:
	go build $(PROJECT)/...

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
	@echo Installing go-1.8 snap
	@sudo snap install go --channel=1.8/stable --classic
	@echo Adding juju PPA for mongodb
	@sudo apt-add-repository --yes ppa:juju/stable
	@sudo apt-get update
	@echo Installing dependencies
	@sudo apt-get --yes install  \
	$(strip $(DEPENDENCIES)) \
	$(shell apt-cache madison juju-mongodb mongodb-server | head -1 | cut -d '|' -f1)

# Install bash_completion
install-etc:
	@echo Installing bash completion
	@sudo install -o root -g root -m 644 etc/bash_completion.d/juju-core /etc/bash_completion.d

GOCHECK_COUNT="$(shell go list -f '{{join .Deps "\n"}}' github.com/juju/juju/... | grep -c "gopkg.in/check.v*")"
check-deps:
	@echo "$(GOCHECK_COUNT) instances of gocheck not in test code"

.PHONY: build check install release-install release-build go-build go-install
.PHONY: clean format simplify
.PHONY: install-dependencies
.PHONY: rebuild-dependencies.tsv
.PHONY: check-deps
.PHONY: add-patches remove-patches