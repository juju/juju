#
# Makefile for juju-core.
#
ifndef GOPATH
$(warning You need to set up a GOPATH.  See the README file.)
endif

PROJECT := github.com/juju/juju
PROJECT_DIR := $(shell go list -e -f '{{.Dir}}' $(PROJECT))
PROJECT_PACKAGES := $(shell go list $(PROJECT)/... | grep -v /vendor/)

# Allow the tests to take longer on arm platforms.
ifeq ($(shell uname -p | sed -E 's/.*(armel|armhf|aarch64).*/golang/'), golang)
	TEST_TIMEOUT := 3000s
else
	TEST_TIMEOUT := 1800s
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
  distro-info-data
  git
  zip
endef

default: build

# Start of GOPATH-dependent targets. Some targets only make sense -
# and will only work - when this tree is found on the GOPATH.
ifeq ($(CURDIR),$(PROJECT_DIR))

ifeq ($(JUJU_SKIP_DEP),true)
dep:
	@echo "skipping dep"
else
$(GOPATH)/bin/dep:
	go get -u github.com/golang/dep/cmd/dep

# populate vendor/ from Gopkg.lock without updating it first (lock file is the single source of truth for machine).
dep: $(GOPATH)/bin/dep
	$(GOPATH)/bin/dep ensure -vendor-only $(verbose)
endif

build: dep go-build

release-build: dep go-build

release-install: dep go-install

pre-check:
	@echo running pre-test checks
	@$(PROJECT_DIR)/scripts/verify.bash

check: dep pre-check
	go test $(CHECK_ARGS) -test.timeout=$(TEST_TIMEOUT) $(PROJECT_PACKAGES) -check.v

install: dep go-install

clean:
	go clean -n -r --cache --testcache $(PROJECT_PACKAGES)

go-install:
	@echo 'go install -ldflags "-s -w" -v $$PROJECT_PACKAGES'
	@go install -ldflags "-s -w" -v $(PROJECT_PACKAGES)

go-build:
	@go build $(PROJECT_PACKAGES)

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

# update Gopkg.lock (if needed), but do not update `vendor/`.
rebuild-dependencies:
	dep ensure -v -no-vendor $(dep-update)

# Install packages required to develop Juju and run tests. The stable
# PPA includes the required mongodb-server binaries.
install-dependencies:
	@echo Installing go-1.11 snap
	@sudo snap install go --channel=1.11/stable --classic
	@echo Adding juju PPA for mongodb
	@sudo apt-add-repository --yes ppa:juju/stable
	@sudo apt-get update
	@echo Installing dependencies
	@sudo apt-get --yes install  \
	$(strip $(DEPENDENCIES)) \
	$(shell apt-cache madison mongodb-server-core juju-mongodb3.2 juju-mongodb mongodb-server | head -1 | cut -d '|' -f1)

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

# CAAS related targets
DOCKER_USERNAME?=jujusolutions
JUJUD_STAGING_DIR?=/tmp/jujud-operator
JUJUD_BIN_DIR=${GOPATH}/bin
OPERATOR_IMAGE_TAG = $(shell jujud version | rev | cut -d- -f3- | rev)

operator-image: install caas/jujud-operator-dockerfile caas/jujud-operator-requirements.txt
	rm -rf ${JUJUD_STAGING_DIR}
	mkdir ${JUJUD_STAGING_DIR}
	cp ${JUJUD_BIN_DIR}/jujud ${JUJUD_STAGING_DIR}
	cp caas/jujud-operator-dockerfile ${JUJUD_STAGING_DIR}
	cp caas/jujud-operator-requirements.txt ${JUJUD_STAGING_DIR}
	docker build -f ${JUJUD_STAGING_DIR}/jujud-operator-dockerfile -t ${DOCKER_USERNAME}/caas-jujud-operator:${OPERATOR_IMAGE_TAG} ${JUJUD_STAGING_DIR}
	rm -rf ${JUJUD_STAGING_DIR}

push-operator-image: operator-image
	docker push ${DOCKER_USERNAME}/caas-jujud-operator

check-k8s-model:
	@:$(if $(value JUJU_K8S_MODEL),, $(error Undefined JUJU_K8S_MODEL))
	@juju show-model ${JUJU_K8S_MODEL} > /dev/null

local-operator-update: check-k8s-model operator-image
	$(eval kubeworkers != juju status -m ${JUJU_K8S_MODEL} kubernetes-worker --format json | jq -c '.machines | keys' | tr  -c '[:digit:]' ' ' 2>&1)
	docker save ${DOCKER_USERNAME}/caas-jujud-operator | gzip > /tmp/caas-jujud-operator-image.tar.gz
	$(foreach wm,$(kubeworkers), juju scp -m ${JUJU_K8S_MODEL} /tmp/caas-jujud-operator-image.tar.gz $(wm):/tmp/caas-jujud-operator-image.tar.gz ; )
	$(foreach wm,$(kubeworkers), juju ssh -m ${JUJU_K8S_MODEL} $(wm) -- "zcat /tmp/caas-jujud-operator-image.tar.gz | docker load" ; )

.PHONY: build check install release-install release-build go-build go-install
.PHONY: clean format simplify
.PHONY: install-dependencies
.PHONY: rebuild-dependencies
.PHONY: dep check-deps
