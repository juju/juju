#
# Makefile for juju-core.
#
PROJECT_DIR := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))
PROJECT := github.com/juju/juju

BUILD_DIR ?= $(PROJECT_DIR)/_build

define MAIN_PACKAGES
  github.com/juju/juju/cmd/juju
  github.com/juju/juju/cmd/jujuc
  github.com/juju/juju/cmd/jujud
  github.com/juju/juju/cmd/plugins/juju-metadata
endef

# Allow the tests to take longer on restricted platforms.
ifeq ($(shell uname -p | sed -E 's/.*(armel|armhf|aarch64|ppc64le|ppc64|s390x).*/golang/'), golang)
	TEST_TIMEOUT := 5400s
else
	TEST_TIMEOUT := 1800s
endif

# Limit concurrency on s390x.
ifeq ($(shell uname -p | sed -E 's/.*(s390x).*/golang/'), golang)
	TEST_ARGS := -p 4
else
	TEST_ARGS := 
endif

# Enable verbose testing for reporting.
ifeq ($(VERBOSE_CHECK), 1)
	CHECK_ARGS = -v $(TEST_ARGS)
else
	CHECK_ARGS = $(TEST_ARGS)
endif

GIT_COMMIT ?= $(shell git -C $(PROJECT_DIR) rev-parse HEAD 2>/dev/null)
# If .git directory is missing, we are building out of an archive, otherwise report
# if the tree that is checked out is dirty (modified) or clean.
GIT_TREE_STATE = $(if $(shell git -C $(PROJECT_DIR) rev-parse --is-inside-work-tree 2>/dev/null | grep -e 'true'),$(if $(shell git -C $(PROJECT_DIR) status --porcelain),dirty,clean),archive)

# Build tags passed to go install/build.
# Example: BUILD_TAGS="minimal provider_kubernetes"
BUILD_TAGS ?= 

# Build number passed in must be a monotonic int representing
# the build.
JUJU_BUILD_NUMBER ?= 

# Build flag passed to go -mod
# CI should set this to vendor
JUJU_GOMOD_MODE ?= mod

# Compile with debug flags if requested.
ifeq ($(DEBUG_JUJU), 1)
    COMPILE_FLAGS = -gcflags "all=-N -l"
    LINK_FLAGS = -ldflags "-X $(PROJECT)/version.GitCommit=$(GIT_COMMIT) -X $(PROJECT)/version.GitTreeState=$(GIT_TREE_STATE) -X $(PROJECT)/version.build=$(JUJU_BUILD_NUMBER)"
else
    COMPILE_FLAGS =
    LINK_FLAGS = -ldflags "-s -w -X $(PROJECT)/version.GitCommit=$(GIT_COMMIT) -X $(PROJECT)/version.GitTreeState=$(GIT_TREE_STATE) -X $(PROJECT)/version.build=$(JUJU_BUILD_NUMBER)"
endif

define DEPENDENCIES
  ca-certificates
  bzip2
  distro-info-data
  git
  zip
endef

default: build

.PHONY: help
help:
	@echo "Usage: \n"
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | sort | column -t -s ':' |  sed -e 's/^/ /'

build: rebuild-schema go-build
## build: Create Juju binaries

release-build: go-build
## release-build: Construct Juju binaries, without building schema

release-install: go-install
## release-install: Install Juju binaries

pre-check:
## pre-check: Verify go code via static analysis
	@echo running pre-test checks
	@INCLUDE_GOLINTERS=1 $(PROJECT_DIR)/scripts/verify.bash

check: pre-check run-tests
## check: Verify Juju code using static analysis and unit tests

test: run-tests
## test: Verify Juju code using unit tests

# Can't make the length of the TMP dir too long or it hits socket name length issues.
run-tests:
## run-tests: Run the unit tests
	$(eval TMP := $(shell mktemp -d jj-XXX --tmpdir))
	$(eval TEST_PACKAGES := $(shell go list $(PROJECT)/... | grep -v $(PROJECT)$$ | grep -v $(PROJECT)/vendor/ | grep -v $(PROJECT)/acceptancetests/ | grep -v $(PROJECT)/generate/ | grep -v mocks))
	@echo 'go test -mod=$(JUJU_GOMOD_MODE) -tags "$(BUILD_TAGS)" $(CHECK_ARGS) -test.timeout=$(TEST_TIMEOUT) $$TEST_PACKAGES -check.v'
	@TMPDIR=$(TMP) go test -mod=$(JUJU_GOMOD_MODE) -tags "$(BUILD_TAGS)" $(CHECK_ARGS) -test.timeout=$(TEST_TIMEOUT) $(TEST_PACKAGES) -check.v
	@rm -r $(TMP)

install: rebuild-schema go-install
## install: Install Juju binaries

clean:
## clean: Clean the cache and test caches
	go clean -n -r --cache --testcache $(PROJECT)/...

go-install:
## go-install: Install Juju binaries without updating dependencies
	@echo 'go install -mod=$(JUJU_GOMOD_MODE) -tags "$(BUILD_TAGS)" $(COMPILE_FLAGS) $(LINK_FLAGS) -v $$MAIN_PACKAGES'
	@go install -mod=$(JUJU_GOMOD_MODE) -tags "$(BUILD_TAGS)" $(COMPILE_FLAGS) $(LINK_FLAGS) -v $(strip $(MAIN_PACKAGES))

go-build:
## go-build: Build Juju binaries without updating dependencies
	@mkdir -p $(BUILD_DIR)/bin
	@echo 'go build -mod=$(JUJU_GOMOD_MODE) -o $(BUILD_DIR)/bin -tags "$(BUILD_TAGS)" $(COMPILE_FLAGS) $(LINK_FLAGS) -v $$MAIN_PACKAGES'
	@go build -mod=$(JUJU_GOMOD_MODE) -o $(BUILD_DIR)/bin -tags "$(BUILD_TAGS)" $(COMPILE_FLAGS) $(LINK_FLAGS) -v $(strip $(MAIN_PACKAGES))

vendor-dependencies:
## vendor-dependencies: updates vendored dependencies
	@go mod vendor

# Reformat source files.
format:
## format: Format the go source code
	gofmt -w -l .

# Reformat and simplify source files.
simplify:
## simplify: Format and simplify the go source code
	gofmt -w -l -s .

rebuild-schema:
## rebuild-schema: Rebuild the schema for clients with the latest facades
	@echo "Generating facade schema..."
ifdef SCHEMA_PATH
	@go run $(PROJECT)/generate/schemagen "$(SCHEMA_PATH)"
else
	@go run $(PROJECT)/generate/schemagen \
		./apiserver/facades/schema.json
endif

# Install packages required to develop Juju and run tests. The stable
# PPA includes the required mongodb-server binaries.
install-snap-dependencies:
## install-snap-dependencies: Install the supported snap dependencies
	@echo Installing go-1.14 snap
	@sudo snap install go --channel=1.14/stable --classic

install-mongo-dependencies:
## install-mongo-dependencies: Install Mongo and its dependencies
	@echo Adding juju PPA for mongodb
	@sudo apt-add-repository --yes ppa:juju/stable
	@sudo apt-get update
	@echo Installing dependencies
	@sudo apt-get --yes install  \
	$(strip $(DEPENDENCIES)) \
	$(shell apt-cache madison mongodb-server-core juju-mongodb3.2 juju-mongodb mongodb-server | head -1 | cut -d '|' -f1)

install-dependencies: install-snap-dependencies install-mongo-dependencies
## install-dependencies: Install all the dependencies
	@echo "Installing dependencies"

# Install bash_completion
install-etc:
## install-etc: Install auto-completion
	@echo Installing bash completion
	@sudo install -o root -g root -m 644 etc/bash_completion.d/juju /usr/share/bash-completion/completions
	@sudo install -o root -g root -m 644 etc/bash_completion.d/juju-version /usr/share/bash-completion/completions

setup-lxd:
## setup-lxd: Auto configure LXD
ifeq ($(shell ifconfig lxdbr0 2>&1 | grep -q "inet addr" && echo true),true)
	@echo IPv4 networking is already setup for LXD.
	@echo run "sudo scripts/setup-lxd.sh" to reconfigure IPv4 networking
else
	@echo Setting up IPv4 networking for LXD
	@sudo scripts/setup-lxd.sh || true
endif


GOCHECK_COUNT="$(shell go list -f '{{join .Deps "\n"}}' ${PROJECT}/... | grep -c "gopkg.in/check.v*")"
check-deps:
## check-deps: Check dependencies are correct versions
	@echo "$(GOCHECK_COUNT) instances of gocheck not in test code"


# CAAS related targets
DOCKER_USERNAME            ?= jujusolutions
DOCKER_STAGING_DIR         ?= ${BUILD_DIR}/docker-staging
JUJUD_STAGING_DIR          ?= ${DOCKER_STAGING_DIR}/jujud-operator
JUJUD_BIN_DIR              ?= ${BUILD_DIR}/bin
OPERATOR_IMAGE_BUILD_SRC   ?= true

# Import shell functions from make_functions.sh
BUILD_OPERATOR_IMAGE=sh -c '. "${PROJECT_DIR}/make_functions.sh"; build_operator_image "$$@"' build_operator_image
OPERATOR_IMAGE_PATH=sh -c '. "${PROJECT_DIR}/make_functions.sh"; operator_image_path "$$@"' operator_image_path
OPERATOR_IMAGE_LEGACY_PATH=sh -c '. "${PROJECT_DIR}/make_functions.sh"; operator_image_legacy_path "$$@"' operator_image_legacy_path

operator-check-build:
ifeq ($(OPERATOR_IMAGE_BUILD_SRC),true)
	make build
else
	@echo "skipping to build jujud bin, use existing one at ${JUJUD_BIN_DIR}/jujud."
endif

operator-image: operator-check-build
## operator-image: Build operator image via docker
	$(BUILD_OPERATOR_IMAGE)

push-operator-image: operator-image
## push-operator-image: Push up the new built operator image via docker
	docker push "$(shell ${OPERATOR_IMAGE_PATH})"
	@if [ "$(shell ${OPERATOR_IMAGE_PATH})" != "$(shell ${OPERATOR_IMAGE_LEGACY_PATH})" ]; then \
		docker push "$(shell ${OPERATOR_IMAGE_LEGACY_PATH})"; \
	fi

microk8s-operator-update: install operator-image
## microk8s-operator-update: Push up the new built operator image for use with microk8s
	docker save "$(shell ${OPERATOR_IMAGE_PATH})" | microk8s.ctr --namespace k8s.io image import -

check-k8s-model:
## check-k8s-model: Check if k8s model is present in show-model
	@:$(if $(value JUJU_K8S_MODEL),, $(error Undefined JUJU_K8S_MODEL))
	@juju show-model ${JUJU_K8S_MODEL} > /dev/null

local-operator-update: check-k8s-model operator-image
## local-operator-update: Build then update local operator image
	$(eval kubeworkers != juju status -m ${JUJU_K8S_MODEL} kubernetes-worker --format json | jq -c '.machines | keys' | tr  -c '[:digit:]' ' ' 2>&1)
	docker save "$(shell ${OPERATOR_IMAGE_PATH})" | gzip > ${DOCKER_STAGING_DIR}/jujud-operator-image.tar.gz
	$(foreach wm,$(kubeworkers), juju scp -m ${JUJU_K8S_MODEL} ${DOCKER_STAGING_DIR}/jujud-operator-image.tar.gz $(wm):/tmp/jujud-operator-image.tar.gz ; )
	$(foreach wm,$(kubeworkers), juju ssh -m ${JUJU_K8S_MODEL} $(wm) -- "zcat /tmp/jujud-operator-image.tar.gz | docker load" ; )

STATIC_ANALYSIS_JOB ?= 

static-analysis:
## static-analysis: Check the go code using static-analysis
	@cd tests && ./main.sh static_analysis ${STATIC_ANALYSIS_JOB}

.PHONY: build check install release-install release-build go-build go-install
.PHONY: clean format simplify test run-tests
.PHONY: install-dependencies
.PHONY: check-deps
