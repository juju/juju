.PHONY: help
help:
	@echo "Usage: \n"
	@sed -n 's/^## //p' ${MAKEFILE_LIST} | sort | column -t -s ':' |  sed -e 's/^/ /'

# Export this first, incase we want to change it in the included makefiles.
export CGO_ENABLED=0

# Makefile for juju-core.
#
PROJECT_DIR := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))
PROJECT := github.com/juju/juju

GOOS=$(shell go env GOOS)
GOARCH=$(shell go env GOARCH)
GOHOSTOS=$(shell go env GOHOSTOS)
GOHOSTARCH=$(shell go env GOHOSTARCH)
GO_MOD_VERSION=$(shell grep "^go" go.mod | awk '{print $$2}')
GO_INSTALLED_VERSION=$(shell go version | awk '{print $$3}' | sed -e /.*go/s///)

# Build number passed in must be a monotonic int representing
# the build.
JUJU_BUILD_NUMBER ?=

# JUJU_VERSION is the JUJU version currently being represented in this
# repository.
JUJU_VERSION=$(shell go run -ldflags "-X $(PROJECT)/version.build=$(JUJU_BUILD_NUMBER)" version/helper/main.go)

# BUILD_DIR is the directory relative to this project where we place build
# artifacts created by this Makefile.
BUILD_DIR ?= $(PROJECT_DIR)/_build
BIN_DIR ?= ${BUILD_DIR}/${GOOS}_${GOARCH}/bin

# JUJU_METADATA_SOURCE is the directory where we place simple streams archives
# for built juju binaries.
JUJU_METADATA_SOURCE ?= ${BUILD_DIR}/simplestreams

# TEST_PACKAGE_LIST is the path to a file that is a newline delimited list of
# packages to test. This file must be sorted.
TEST_PACKAGE_LIST ?=

# bin_platform_path calculates the bin directory path for build artifacts for
# the list of Go style platforms passed to this macro. For example
# linux/amd64 linux/arm64
bin_platform_paths = $(addprefix ${BUILD_DIR}/, $(addsuffix /bin, $(subst /,_,${1})))

# tool_platform_paths takes a juju binary to be built and the platform that it
# is to be built for and returns a list of paths for that binary to be output.
tool_platform_paths = $(addsuffix /${1},$(call bin_platform_paths,${2}))

# simplestream_paths takes a list of Go style platforms to calculate the
# paths to their respective simplestreams agent binary archives.
simplestream_paths = $(addprefix ${JUJU_METADATA_SOURCE}/, $(addprefix tools/released/juju-${JUJU_VERSION}-, $(addsuffix .tgz,$(subst /,-,${1}))))

# CLIENT_PACKAGE_PLATFORMS defines a white space seperated list of platforms
# to build the Juju client binaries for. Platforms are defined as GO style
# OS_ARCH.
CLIENT_PACKAGE_PLATFORMS ?= $(GOOS)/$(GOARCH)

# AGENT_PACKAGE_PLATFORMS defines a white space seperated list of platforms
# to build the Juju agent binaries for. Platforms are defined as GO style
# OS_ARCH.
AGENT_PACKAGE_PLATFORMS ?= $(GOOS)/$(GOARCH)

# OCI_IMAGE_PLATFORMS defines a white space seperated list of platforms
# to build the Juju OCI images for. Platforms are defined as GO style
# OS_ARCH.
OCI_IMAGE_PLATFORMS ?= linux/$(GOARCH)

# Build tags passed to go install/build.
# Example: BUILD_TAGS="minimal provider_kubernetes"
BUILD_TAGS ?=

# EXTRA_BUILD_TAGS is not passed in, but built up from context.
EXTRA_BUILD_TAGS =
# Enable coverage collection.
ifneq ($(COVERAGE_COLLECT_URL),)
    EXTRA_BUILD_TAGS += cover
endif

# FINAL_BUILD_TAGS is the final list of build tags.
FINAL_BUILD_TAGS=$(shell echo "$(BUILD_TAGS) $(EXTRA_BUILD_TAGS)" | awk '{$$1=$$1};1' | tr ' ' ',')

# GIT_COMMIT the current git commit of this repository
GIT_COMMIT ?= $(shell git -C $(PROJECT_DIR) rev-parse HEAD 2>/dev/null)

# Build flag passed to go -mod defaults to readonly to support go workspaces.
# CI should set this to vendor
JUJU_GOMOD_MODE ?= readonly

# If .git directory is missing, we are building out of an archive, otherwise report
# if the tree that is checked out is dirty (modified) or clean.
GIT_TREE_STATE = $(if $(shell git -C $(PROJECT_DIR) rev-parse --is-inside-work-tree 2>/dev/null | grep -e 'true'),$(if $(shell git -C $(PROJECT_DIR) status --porcelain),dirty,clean),archive)

# BUILD_AGENT_TARGETS is a list of make targets that get built, that fall under
# the category of Juju agents. These targets are also the ones
# we are more then likely wanting to cross compile.
# NOTES:
# - We filter pebble here for only linux builds as that is only what it will
#   compile for at the moment.
define BUILD_AGENT_TARGETS
	$(call tool_platform_paths,jujuc,${AGENT_PACKAGE_PLATFORMS}) \
	$(call tool_platform_paths,jujud,${AGENT_PACKAGE_PLATFORMS}) \
	$(call tool_platform_paths,containeragent,$(filter-out windows%,${AGENT_PACKAGE_PLATFORMS})) \
	$(call tool_platform_paths,pebble,$(filter linux%,${AGENT_PACKAGE_PLATFORMS}))
endef

# BUILD_CLIENT_TARGETS is a list of make targets that get built that fall under
# the category of Juju clients. These targets are also less likely to be cross
# compiled
define BUILD_CLIENT_TARGETS
	$(call tool_platform_paths,juju,${CLIENT_PACKAGE_PLATFORMS}) \
	$(call tool_platform_paths,juju-metadata,${CLIENT_PACKAGE_PLATFORMS}) \
	$(call tool_platform_paths,juju-wait-for,${CLIENT_PACKAGE_PLATFORMS})
endef

# SIMPLESTREAMS_TARGETS is a list of make targets that get built when a
# user asks for simplestreams to be built. Because simplestreams are mainly
# mainly concerned with that of packaging juju agent binaries we work off of
# the Go style platforms.
define SIMPLESTREAMS_TARGETS
	$(call simplestream_paths,${AGENT_PACKAGE_PLATFORMS})
endef

# INSTALL_TARGETS is a list of make targets that get installed when make
# install is run.
define INSTALL_TARGETS
	juju \
	jujuc \
	jujud \
	containeragent \
	juju-metadata \
	juju-wait-for
endef

# Windows doesn't support the agent binaries
ifeq ($(GOOS), windows)
    INSTALL_TARGETS = juju \
                      juju-metadata \
                      juju-wait-for
endif

# We only add pebble to the list of install targets if we are building for linux
ifeq ($(GOOS), linux)
	INSTALL_TARGETS += pebble
endif

# Allow the tests to take longer on restricted platforms.
ifeq ($(shell echo "${GOARCH}" | sed -E 's/.*(arm|arm64|ppc64le|ppc64|s390x).*/golang/'), golang)
    TEST_TIMEOUT ?= 5400s
else
    TEST_TIMEOUT ?= 2700s
endif
TEST_TIMEOUT:=$(TEST_TIMEOUT)

TEST_ARGS ?=
# Limit concurrency on s390x.
ifeq ($(shell echo "${GOARCH}" | sed -E 's/.*(s390x).*/golang/'), golang)
	TEST_ARGS += -p 4
endif

# Enable verbose testing for reporting.
ifeq ($(VERBOSE_CHECK), 1)
	CHECK_ARGS = -v
endif

define link_flags_version
-X $(PROJECT)/version.GitCommit=$(GIT_COMMIT) \
-X $(PROJECT)/version.GitTreeState=$(GIT_TREE_STATE) \
-X $(PROJECT)/version.build=$(JUJU_BUILD_NUMBER) \
-X $(PROJECT)/version.GoBuildTags=$(FINAL_BUILD_TAGS) \
-X $(PROJECT)/internal/debug/coveruploader.putURL=$(COVERAGE_COLLECT_URL)
endef

# Enable coverage collection.
ifneq ($(COVERAGE_COLLECT_URL),)
    COVER_COMPILE_FLAGS = -cover -covermode=atomic
    COVER_LINK_FLAGS = -checklinkname=0
endif

# Compile with debug flags if requested.
ifeq ($(DEBUG_JUJU), 1)
    COMPILE_FLAGS = $(COVER_COMPILE_FLAGS) -gcflags "all=-N -l"
    LINK_FLAGS = "$(COVER_LINK_FLAGS) $(link_flags_version)"
else
    COMPILE_FLAGS = $(COVER_COMPILE_FLAGS)
    LINK_FLAGS = "$(COVER_LINK_FLAGS) -s -w -extldflags '-static' $(link_flags_version)"
endif

define DEPENDENCIES
  ca-certificates
  bzip2
  distro-info-data
  git
  zip
endef

# run_go_build is a canned command sequence for the steps required to build a
# juju package. It's expected that the make target using this sequence has a
# local variable defined for PACKAGE. An example of PACKAGE would be
# PACKAGE=github.com/juju/juju
# 
# This canned command also allows building for architectures defined as
# ppc64el. Because of legacy Juju we use the arch ppc64el over the go defined
# arch of ppc64le. This canned command will do a last minute transformation of
# the string we build the "correct" go architecture. However the build result
# will still be placed at the expected location with names matching ppc64el.
define run_go_build
	$(eval OS = $(word 1,$(subst _, ,$*)))
	$(eval ARCH = $(word 2,$(subst _, ,$*)))
	$(eval BBIN_DIR = ${BUILD_DIR}/${OS}_${ARCH}/bin)
	$(eval BUILD_ARCH = $(subst ppc64el,ppc64le,${ARCH}))
	@@mkdir -p ${BBIN_DIR}
	@echo "Building ${PACKAGE} for ${OS}/${ARCH}"
	@env GOOS=${OS} \
		GOARCH=${BUILD_ARCH} \
		go build \
			-mod=$(JUJU_GOMOD_MODE) \
			-tags=$(FINAL_BUILD_TAGS) \
			-o ${BBIN_DIR} \
			$(COMPILE_FLAGS) \
			-ldflags $(LINK_FLAGS) \
			-v ${PACKAGE}
endef

define run_go_install
	@echo "Installing ${PACKAGE}"
	@go install \
		-mod=$(JUJU_GOMOD_MODE) \
		-tags=$(FINAL_BUILD_TAGS) \
		$(COMPILE_FLAGS) \
		-ldflags $(LINK_FLAGS) \
		-v ${PACKAGE}
endef

default: build

.PHONY: juju
juju: PACKAGE = github.com/juju/juju/cmd/juju
juju:
## juju: Install juju without updating dependencies
	${run_go_install}

.PHONY: jujuc
jujuc: PACKAGE = github.com/juju/juju/cmd/jujuc
jujuc:
## jujuc: Install jujuc without updating dependencies
	${run_go_install}

.PHONY: jujud
jujud: PACKAGE = github.com/juju/juju/cmd/jujud
jujud:
## jujud: Install jujud without updating dependencies
	${run_go_install}

.PHONY: containeragent
containeragent: PACKAGE = github.com/juju/juju/cmd/containeragent
containeragent:
## containeragent: Install containeragent without updating dependencies
	${run_go_install}

.PHONY: juju-metadata
juju-metadata: PACKAGE = github.com/juju/juju/cmd/plugins/juju-metadata
juju-metadata:
## juju-metadata: Install juju-metadata without updating dependencies
	${run_go_install}

.PHONY: juju-wait-for
juju-wait-for: PACKAGE = github.com/juju/juju/cmd/plugins/juju-wait-for
juju-wait-for:
## juju-wait-for: Install juju-wait-for without updating dependencies
	${run_go_install}

.PHONY: pebble
pebble: PACKAGE = github.com/canonical/pebble/cmd/pebble
pebble:
## pebble: Install pebble without updating dependencies
	${run_go_install}

.PHONY: phony_explicit
phony_explicit:
# phone_explicit: is a dummy target that can be added to pattern targets to phony make.

${BUILD_DIR}/%/bin/juju: PACKAGE = github.com/juju/juju/cmd/juju
${BUILD_DIR}/%/bin/juju: phony_explicit
# build for juju
	$(run_go_build)

${BUILD_DIR}/%/bin/jujuc: PACKAGE = github.com/juju/juju/cmd/jujuc
${BUILD_DIR}/%/bin/jujuc: phony_explicit
# build for jujuc
	$(run_go_build)

${BUILD_DIR}/%/bin/jujud: PACKAGE = github.com/juju/juju/cmd/jujud
${BUILD_DIR}/%/bin/jujud: phony_explicit
# build for jujud
	$(run_go_build)

${BUILD_DIR}/%/bin/containeragent: PACKAGE = github.com/juju/juju/cmd/containeragent
${BUILD_DIR}/%/bin/containeragent: phony_explicit
# build for containeragent
	$(run_go_build)

${BUILD_DIR}/%/bin/juju-metadata: PACKAGE = github.com/juju/juju/cmd/plugins/juju-metadata
${BUILD_DIR}/%/bin/juju-metadata: phony_explicit
# build for juju-metadata
	$(run_go_build)

${BUILD_DIR}/%/bin/juju-wait-for: PACKAGE = github.com/juju/juju/cmd/plugins/juju-wait-for
${BUILD_DIR}/%/bin/juju-wait-for: phony_explicit
# build for juju-wait-for
	$(run_go_build)

${BUILD_DIR}/%/bin/pebble: PACKAGE = github.com/canonical/pebble/cmd/pebble
${BUILD_DIR}/%/bin/pebble: phony_explicit
# build for pebble
	$(run_go_build)

${JUJU_METADATA_SOURCE}/tools/released/juju-${JUJU_VERSION}-%.tgz: phony_explicit juju go-agent-build
	@echo "Packaging simplestream tools for juju ${JUJU_VERSION} on $*"
	@mkdir -p ${JUJU_METADATA_SOURCE}/tools/released
	@tar czf "$@" -C $(call bin_platform_paths,$(subst -,/,$*)) jujud jujuc

.PHONY: simplestreams
simplestreams: juju juju-metadata ${SIMPLESTREAMS_TARGETS}
	@juju metadata generate-agents -d ${JUJU_METADATA_SOURCE} --clean --prevent-fallback ;
	@echo "\nRun export JUJU_METADATA_SOURCE=\"${JUJU_METADATA_SOURCE}\" if not defined in your env"

.PHONY: build
build: rebuild-schema go-build
## build: builds all the targets including rebuilding a new schema.

.PHONY: go-agent-build
go-agent-build: $(BUILD_AGENT_TARGETS)

.PHONY: go-client-build
go-client-build: $(BUILD_CLIENT_TARGETS)

.PHONY: go-build
go-build: go-agent-build go-client-build
## go-build: builds all the targets without rebuilding a new schema.

.PHONY: release-build
release-build: go-agent-build
## release-build: Construct Juju binaries, without building schema

.PHONY: release-install
release-install: $(INSTALL_TARGETS)
## release-install: Install Juju binaries

.PHONY: pre-check
pre-check:
## pre-check: Verify go code via static analysis
	@echo running pre-test checks
	@INCLUDE_GOLINTERS=1 $(PROJECT_DIR)/scripts/verify.bash

.PHONY: check
check: pre-check run-tests
## check: Verify Juju code using static analysis and unit tests

.PHONY: test
test: run-tests
## test: Verify Juju code using unit tests

.PHONY: race-test
race-test:
## race-test: Verify Juju code using unit tests with the race detector enabled
	+make run-tests CGO_ENABLED=1 TEST_ARGS="$(TEST_ARGS) -race"

.PHONY: cover-test
cover-test:
	+make run-tests TEST_ARGS="$(TEST_ARGS) -cover -covermode=atomic" TEST_EXTRA_ARGS="$(TEST_EXTRA_ARGS) -test.gocoverdir=${GOCOVERDIR}"

.PHONY: run-tests run-go-tests go-test-alias
# Can't make the length of the TMP dir too long or it hits socket name length issues.
run-tests:
## run-tests: Run the unit tests
	$(eval OS = $(shell go env GOOS))
	$(eval ARCH = $(shell go env GOARCH))
	$(eval BUILD_ARCH = $(subst ppc64el,ppc64le,${ARCH}))
	$(eval TMP := $(shell mktemp -d $${TMPDIR:-/tmp}/jj-XXX))
	$(eval TEST_PACKAGES := $(shell make -s test-packages))
	@echo 'go test -mod=$(JUJU_GOMOD_MODE) -tags=$(FINAL_BUILD_TAGS) $(TEST_ARGS) $(CHECK_ARGS) -test.timeout=$(TEST_TIMEOUT) $$TEST_PACKAGES -check.v $(TEST_EXTRA_ARGS)'
	@TMPDIR=$(TMP) \
		go test -v -mod=$(JUJU_GOMOD_MODE) -tags=$(FINAL_BUILD_TAGS) $(TEST_ARGS) $(CHECK_ARGS) -test.timeout=$(TEST_TIMEOUT) $(TEST_PACKAGES) -check.v $(TEST_EXTRA_ARGS)
	@rm -r $(TMP)

.PHONY: test-packages
test-packages:
## test-packages: List all the packages that should be tested
# How this line selects packages to test:
# 1. List all the project packages with json output.
# 2. Filter out packages without test files and select their package import path.
# 3. Sort the list for comm.
# 4. If there is a list of packages in TEST_PACKAGE_LIST, use it as a filter.
# 5. Filter out vendored packages.
# 6. Filter out packages in the generate directory.
# 7. Filter out packages in the mocks directory.
# 8. Filter out all mocks.
	@go list -json $(PROJECT)/... | jq -s -r '[.[] | if (.TestGoFiles | length) + (.XTestGoFiles | length) > 0 then .ImportPath else null end]|del(..|nulls).[]' | sort | ([ -f "$(TEST_PACKAGE_LIST)" ] && comm -12 "$(TEST_PACKAGE_LIST)" - || cat) | grep -v $(PROJECT)$$ | grep -v $(PROJECT)/vendor/ | grep -v $(PROJECT)/generate/ | grep -v $(PROJECT)/mocks/ | grep -v mocks

.PHONY: install
install: rebuild-schema go-install
## install: Install Juju binaries with a rebuilt schema

.PHONY: go-install
go-install: $(INSTALL_TARGETS)
## go-install: Install Juju binaries

.PHONY: clean
clean:
## clean: Clean the cache and test caches
	go clean -x --cache --testcache
	go clean -x -r $(PROJECT)/...

.PHONY: vendor-dependencies
vendor-dependencies:
## vendor-dependencies: updates vendored dependencies
	@go mod vendor

.PHONY: format
# Reformat source files.
format:
## format: Format the go source code
	gofmt -w -l .

.PHONY: simplify
# Reformat and simplify source files.
simplify:
## simplify: Format and simplify the go source code
	gofmt -w -l -s .

.PHONY: rebuild-schema
rebuild-schema:
## rebuild-schema: Rebuild the schema for clients with the latest facades
	@echo "Generating facade schema..."
# GOOS and GOARCH environment variables are cleared in case the user is trying to cross architecture compilation.
ifdef SCHEMA_PATH
	@env GOOS= GOARCH= go run $(PROJECT)/generate/schemagen -admin-facades -facade-group=client "$(SCHEMA_PATH)/schema.json"
else
	@env GOOS= GOARCH= go run $(PROJECT)/generate/schemagen -admin-facades -facade-group=client \
		./apiserver/facades/schema.json
endif

.PHONY: install-snap-dependencies
# Install packages required to develop Juju and run tests. The stable
# PPA includes the required mongodb-server binaries.
install-snap-dependencies:
## install-snap-dependencies: Install the supported snap dependencies
ifeq ($(shell if [ "$(GO_INSTALLED_VERSION)" \> "$(GO_MOD_VERSION)" -o "$(GO_INSTALLED_VERSION)" = "$(GO_MOD_VERSION)" ]; then echo 1; fi),1)
	@echo 'Using installed go-$(GO_MOD_VERSION)'
endif
ifeq ("$(GO_INSTALLED_VERSION)","")
	@echo 'Installing go-$(GO_MOD_VERSION) snap'
	@sudo snap install go --channel=$(GO_MOD_VERSION)/stable --classic
else
ifeq ($(shell if [ "$(GO_INSTALLED_VERSION)" \< "$(GO_MOD_VERSION)" ]; then echo 1; fi),1)
	$(warning "warning: version of go too low: use 'snap refresh go --channel=$(GO_MOD_VERSION)'")
	$(error "error Installed go version '$(GO_INSTALLED_VERSION)' less than required go version '$(GO_MOD_VERSION)'")
endif
endif

WAIT_FOR_DPKG=bash -c '. "${PROJECT_DIR}/make_functions.sh"; wait_for_dpkg "$$@"' wait_for_dpkg
JUJU_DB_VERSION=4.4
JUJU_DB_CHANNEL=${JUJU_DB_VERSION}/stable

.PHONY: install-mongo-dependencies
install-mongo-dependencies:
## install-mongo-dependencies: Install Mongo and its dependencies
	@echo Installing ${JUJU_DB_CHANNEL} juju-db snap for mongodb
	@sudo snap refresh juju-db --channel=${JUJU_DB_CHANNEL} 2> /dev/null; sudo snap install juju-db --channel=${JUJU_DB_CHANNEL} 2> /dev/null
	@$(WAIT_FOR_DPKG)
	@sudo apt-get --yes install  $(strip $(DEPENDENCIES))

.PHONY: install-dependencies
install-dependencies: install-snap-dependencies install-mongo-dependencies
## install-dependencies: Install all the dependencies
	@echo "Installing dependencies"

.PHONY: install-etc
# Install bash_completion
install-etc:
## install-etc: Install auto-completion
	@echo Installing bash completion
	@sudo install -o root -g root -m 644 etc/bash_completion.d/juju /usr/share/bash-completion/completions
	@sudo install -o root -g root -m 644 etc/bash_completion.d/juju-version /usr/share/bash-completion/completions

.PHONY: setup-lxd
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
.PHONY: check-deps
check-deps:
## check-deps: Check dependencies are correct versions
	@echo "$(GOCHECK_COUNT) instances of gocheck not in test code"


# CAAS related targets
export OCI_BUILDER         ?= $(shell (which podman 2>&1 > /dev/null && echo podman) || echo docker )
DOCKER_USERNAME            ?= docker.io/jujusolutions
DOCKER_BUILDX_CONTEXT      ?= juju-make
DOCKER_STAGING_DIR         ?= ${BUILD_DIR}/docker-staging
JUJUD_STAGING_DIR          ?= ${DOCKER_STAGING_DIR}/jujud-operator
JUJUD_BIN_DIR              ?= ${BIN_DIR}
OPERATOR_IMAGE_BUILD_SRC   ?= true

# Import shell functions from make_functions.sh
# For the k8s operator.
BUILD_OPERATOR_IMAGE=bash -c '. "${PROJECT_DIR}/make_functions.sh"; build_push_operator_image "$$@"' build_push_operator_image
OPERATOR_IMAGE_PATH=bash -c '. "${PROJECT_DIR}/make_functions.sh"; operator_image_path "$$@"' operator_image_path
OPERATOR_IMAGE_RELEASE_PATH=bash -c '. "${PROJECT_DIR}/make_functions.sh"; operator_image_release_path "$$@"' operator_image_release_path
UPDATE_MICROK8S_OPERATOR=bash -c '. "${PROJECT_DIR}/make_functions.sh"; microk8s_operator_update "$$@"' microk8s_operator_update
SEED_REPOSITORY=bash -c '. "${PROJECT_DIR}/make_functions.sh"; seed_repository "$$@"' seed_repository

image_check_prereq=image-check-build
ifneq ($(OPERATOR_IMAGE_BUILD_SRC),true)
	image_check_prereq=image-check-build-skip
endif

.PHONY: image-check
image-check: $(image_check_prereq)

.PHONY: image-check-build
image-check-build:
	CLIENT_PACKAGE_PLATFORMS="$(OCI_IMAGE_PLATFORMS)" AGENT_PACKAGE_PLATFORMS="$(OCI_IMAGE_PLATFORMS)" make go-build

.PHONY: image-check-build-skip
image-check-build-skip:
	@echo "skipping to build jujud bin, use existing one at ${JUJUD_BIN_DIR}/."

.PHONY: docker-builder
docker-builder:
## docker-builder: Makes sure that there is a buildx context for building the oci images
ifeq ($(OCI_BUILDER),docker)
	-@docker buildx create --name ${DOCKER_BUILDX_CONTEXT}
endif

.PHONY: image-check
operator-image: image-check docker-builder
## operator-image: Build operator image via docker
	${BUILD_OPERATOR_IMAGE} "$(OCI_IMAGE_PLATFORMS)" "$(PUSH_IMAGE)"

push_operator_image_prereq=push-operator-image-defined
ifeq ($(JUJU_BUILD_NUMBER),)
	push_operator_image_prereq=push-operator-image-undefined
endif

.PHONY: push-operator-image-defined
push-operator-image-defined: PUSH_IMAGE=true
push-operator-image-defined: operator-image

.PHONY: push-operator-image-undefined
push-operator-image-undefined:
	@echo "error Undefined JUJU_BUILD_NUMBER"

.PHONY: push-operator-image
push-operator-image: $(push_operator_image_prereq)
## push-operator-image: Push up the newly built operator image via docker

.PHONY: push-release-operator-image
push-release-operator-image: PUSH_IMAGE=true
push-release-operator-image: operator-image
## push-release-operator-image: Push up the newly built release operator image via docker

.PHONY: seed-repository
seed-repository:
## seed-repository: Copy required juju images from docker.io/jujusolutions
	JUJU_DB_VERSION=$(JUJU_DB_VERSION) $(SEED_REPOSITORY)


.PHONY: host-install
host-install:
## host-install: installs juju for host os/architecture
	+GOOS=$(GOHOSTOS) GOARCH=$(GOHOSTARCH) make juju

.PHONY: minikube-operator-update
minikube-operator-update: host-install operator-image
## minikube-operator-update: Inject the newly built operator image into minikube
	$(OCI_BUILDER) save "$(shell ${OPERATOR_IMAGE_PATH})" | minikube image load --overwrite=true -

.PHONY: microk8s-operator-update
microk8s-operator-update: host-install operator-image
## microk8s-operator-update: Inject the newly built operator image into microk8s
	@${UPDATE_MICROK8S_OPERATOR}

.PHONY: k3s-operator-update
k3s-operator-update: host-install operator-image
## k3s-operator-update: Inject the newly built operator image into k3s
	$(OCI_BUILDER) save "$(shell ${OPERATOR_IMAGE_PATH})" | sudo k3s ctr images import -


.PHONY: check-k8s-model
check-k8s-model:
## check-k8s-model: Check if k8s model is present in show-model
	@:$(if $(value JUJU_K8S_MODEL),, $(error Undefined JUJU_K8S_MODEL))
	@juju show-model ${JUJU_K8S_MODEL} > /dev/null

.PHONY: local-operator-update
local-operator-update: check-k8s-model operator-image
## local-operator-update: Build then update local operator image
	$(eval kubeworkers != juju status -m ${JUJU_K8S_MODEL} kubernetes-worker --format json | jq -c '.machines | keys' | tr  -c '[:digit:]' ' ' 2>&1)
	$(OCI_BUILDER) save "$(shell ${OPERATOR_IMAGE_PATH})" | gzip > ${DOCKER_STAGING_DIR}/jujud-operator-image.tar.gz
	$(foreach wm,$(kubeworkers), juju scp -m ${JUJU_K8S_MODEL} ${DOCKER_STAGING_DIR}/jujud-operator-image.tar.gz $(wm):/tmp/jujud-operator-image.tar.gz ; )
	$(foreach wm,$(kubeworkers), juju ssh -m ${JUJU_K8S_MODEL} $(wm) -- "zcat /tmp/jujud-operator-image.tar.gz | docker load" ; )

STATIC_ANALYSIS_JOB ?=

.PHONY: static-analysis
static-analysis:
## static-analysis: Check the go code using static-analysis
	@cd tests && ./main.sh static_analysis ${STATIC_ANALYSIS_JOB}
