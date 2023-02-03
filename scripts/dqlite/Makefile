SHELL = bash
.ONESHELL:

PROJECT_DIR := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

DQLITE_BUILD_MACHINE ?= $(shell uname -m)
DQLITE_BUILD_ARCH ?= $(shell go env GOARCH)

DQLITE_ARCHIVE_DEPS_PATH=${PROJECT_DIR}/scripts/dqlite
DQLITE_ARCHIVE_NAME=dqlite-deps
DQLITE_ARCHIVE_PATH=${DQLITE_ARCHIVE_DEPS_PATH}/${DQLITE_ARCHIVE_NAME}.tar.bz2

DQLITE_S3_BUCKET=s3://dqlite-static-libs
DQLITE_S3_ARCHIVE_NAME=$(shell date -u +"%Y-%m-%d")-dqlite-deps-${DQLITE_BUILD_ARCH}.tar.bz2
DQLITE_S3_ARCHIVE_PATH=${DQLITE_S3_BUCKET}/${DQLITE_S3_ARCHIVE_NAME}

DQLITE_EXTRACTED_DEPS_PATH=${PROJECT_DIR}/_deps
DQLITE_EXTRACTED_DEPS_ARCHIVE_PATH=${DQLITE_EXTRACTED_DEPS_PATH}/dqlite-deps-${DQLITE_BUILD_ARCH}

MUSL_PATH=${PROJECT_DIR}/_deps/musl-${DQLITE_BUILD_ARCH}
MUSL_BIN_PATH=${MUSL_PATH}/output/bin

${DQLITE_ARCHIVE_PATH}:
	@./scripts/dqlite/scripts/dqlite/build-lxd.sh

dqlite-build-lxd: ${DQLITE_ARCHIVE_PATH}

dqlite-build:
	@./scripts/dqlite/scripts/dqlite/build.sh

# s3 puts here are without an ACL.
# The bucket uses policies that allow GetObject to anyone,
# and PutObject to select Juju team accounts.
dqlite-deps-push: ${DQLITE_ARCHIVE_PATH}
	@echo "DQLITE: Pushing ${DQLITE_S3_ARCHIVE_PATH} to s3"
	aws s3 cp $< ${DQLITE_S3_ARCHIVE_PATH}
	@echo "DQLITE: Pushing latest-dqlite-deps-${DQLITE_BUILD_ARCH}.tar.bz2 to s3"
	aws s3 cp ${DQLITE_S3_ARCHIVE_PATH} ${DQLITE_S3_BUCKET}/latest-dqlite-deps-${DQLITE_BUILD_ARCH}.tar.bz2

dqlite-install:
	@./scripts/dqlite/scripts/dqlite/install.sh

dqlite-install-if-missing:
	@./scripts/dqlite/scripts/dqlite/install-if-missing.sh

musl-install:
	@./scripts/dqlite/scripts/musl/install.sh

musl-install-if-missing:
	@./scripts/dqlite/scripts/musl/install-if-missing.sh

################################################################################
# REPL
#
# Accessing the dqlite repl on a given controller for debugging purposes.
################################################################################

juju-dqlite-repl-deps-on-controller:
	@juju exec -m controller --machine=0 'sudo which rlwrap &>/dev/null || sudo apt-get -y -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" install rlwrap'
	@juju exec -m controller --machine=0 'sudo which socat &>/dev/null || sudo apt-get -y -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" install socat'

juju-dqlite-repl: juju-dqlite-repl-deps-on-controller
	@echo "[+] Connecting to REPL interface to controller machine 0"
	@juju ssh -m controller 0 'sudo rlwrap -H /root/.dqlite_repl.history socat - /var/lib/juju/dqlite/juju.sock'
