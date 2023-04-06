PROJECT_DIR := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

DQLITE_BUILD_MACHINE ?= $(shell uname -m)
DQLITE_BUILD_ARCH ?= $(shell go env GOARCH)

DQLITE_ARCHIVE_DEPS_PATH=${PROJECT_DIR}/scripts/dqlite
DQLITE_ARCHIVE_NAME=dqlite-deps
DQLITE_ARCHIVE_PATH=${DQLITE_ARCHIVE_DEPS_PATH}/${DQLITE_ARCHIVE_NAME}.tar.bz2

DQLITE_EXTRACTED_DEPS_PATH=${PROJECT_DIR}/_deps
DQLITE_EXTRACTED_DEPS_ARCHIVE_PATH=${DQLITE_EXTRACTED_DEPS_PATH}/dqlite-deps-${DQLITE_BUILD_ARCH}

MUSL_PATH=${PROJECT_DIR}/_deps/musl-${DQLITE_BUILD_ARCH}
MUSL_BIN_PATH=${MUSL_PATH}/output/bin

${DQLITE_ARCHIVE_PATH}:
	+@./scripts/dqlite/scripts/dqlite/build-lxd.sh

dqlite-build-lxd: ${DQLITE_ARCHIVE_PATH}

dqlite-build:
	+@./scripts/dqlite/scripts/dqlite/build.sh

dqlite-push: ${DQLITE_ARCHIVE_PATH}
	+@./scripts/dqlite/scripts/dqlite/push.sh

dqlite-install:
	+@./scripts/dqlite/scripts/dqlite/install.sh

dqlite-install-if-missing:
	+@./scripts/dqlite/scripts/dqlite/install-if-missing.sh

musl-install:
	+@./scripts/dqlite/scripts/musl/install.sh

musl-install-if-missing:
	+@./scripts/dqlite/scripts/musl/install-if-missing.sh

repl-install:
	+@./scripts/dqlite/scripts/repl/install.sh

repl:
	+@./scripts/dqlite/scripts/repl/repl.sh
