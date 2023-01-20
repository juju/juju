SHELL = bash
.ONESHELL:

PROJECT_DIR ?= $(GOPATH)/src/github.com/juju/juju

DQLITE_BUILD_IMAGE=ubuntu:22.04
DQLITE_BUILD_CONTAINER=lib-build-server
DQLITE_BUILD_MACHINE ?= $(shell uname -m)
DQLITE_BUILD_ARCH ?= $(shell go env GOARCH)

DQLITE_ARCHIVE_DEPS_PATH=${PROJECT_DIR}/scripts/dqlite
DQLITE_ARCHIVE_NAME=dqlite-deps
DQLITE_ARCHIVE_PATH=${DQLITE_ARCHIVE_DEPS_PATH}/${DQLITE_ARCHIVE_NAME}.tar.bz2

DQLITE_S3_BUCKET=s3://dqlite-static-libs
DQLITE_S3_ARCHIVE_NAME=$(shell date -u +"%Y-%m-%d")-dqlite-deps-${DQLITE_BUILD_ARCH}.tar.bz2
DQLITE_S3_ARCHIVE_PATH=${DQLITE_S3_BUCKET}/${DQLITE_S3_ARCHIVE_NAME}

DQLITE_EXTRACTED_DEPS_PATH=${PROJECT_DIR}/_deps
DQLITE_EXTRACTED_DEPS_ARCHIVE_PATH=${DQLITE_EXTRACTED_DEPS_PATH}/juju-dqlite-static-lib-deps

# Versions for Dqlite and upstream dependencies.
TAG_LIBTIRPC=upstream/1.3.3
TAG_LIBNSL=v2.0.0
TAG_LIBUV=v1.44.2
TAG_LIBLZ4=v1.9.4
TAG_RAFT=v0.16.0
TAG_SQLITE=version-3.40.0
TAG_DQLITE=v1.12.0

.PHONY: install-deps-on-controller build-and-install-jujud

${DQLITE_ARCHIVE_PATH}:
	@lxc delete -f ${DQLITE_BUILD_CONTAINER} &>/dev/null || true

	@echo "DQLITE: Creating build box using ${DQLITE_BUILD_IMAGE}"
	@lxc launch ${DQLITE_BUILD_IMAGE} ${DQLITE_BUILD_CONTAINER}

	@echo "DQLITE: Waiting for ${DQLITE_BUILD_CONTAINER} to become ready..."
	@lxc exec ${DQLITE_BUILD_CONTAINER} -- bash -c 'while [ "$$(systemctl is-system-running 2>/dev/null)" != "running" ] && [ "$$(systemctl is-system-running 2>/dev/null)" != "degraded" ]; do :; done'

	@echo "DQLITE: Pushing provisioning script to ${DQLITE_BUILD_CONTAINER}"
	@lxc file push ./scripts/dqlite/build-script.sh ${DQLITE_BUILD_CONTAINER}/root/build-script.sh

	@echo "DQLITE: Executing build script"
	@lxc exec -t ${DQLITE_BUILD_CONTAINER} \
		--env TAG_LIBTIRPC=${TAG_LIBTIRPC} \
		--env TAG_LIBNSL=${TAG_LIBNSL} \
		--env TAG_LIBUV=${TAG_LIBUV} \
		--env TAG_LIBLZ4=${TAG_LIBLZ4} \
		--env TAG_RAFT=${TAG_RAFT} \
		--env TAG_SQLITE=${TAG_SQLITE} \
		--env TAG_DQLITE=${TAG_DQLITE} \
			bash /root/build-script.sh

	@echo "DQLITE: Pulling lib archive"
	@lxc file pull ${DQLITE_BUILD_CONTAINER}/root/build/juju-dqlite-static-lib-deps.tar.bz2 $@
	@lxc delete -f $(DQLITE_BUILD_CONTAINER)

# Just make it easy to build dqlite.
dqlite-build: ${DQLITE_ARCHIVE_PATH}

# Build dqlite and its dependencies locally.
dqlite-local-build:
	@echo "DQLITE: Executing build script"
	TAG_LIBTIRPC=${TAG_LIBTIRPC} \
	TAG_LIBNSL=${TAG_LIBNSL} \
	TAG_LIBUV=${TAG_LIBUV} \
	TAG_LIBLZ4=${TAG_LIBLZ4} \
	TAG_RAFT=${TAG_RAFT} \
	TAG_SQLITE=${TAG_SQLITE} \
	TAG_DQLITE=${TAG_DQLITE} \
		./scripts/dqlite/build-script.sh

	@echo "DQLITE: Placing lib archive"
	@mv ./scripts/dqlite/dqlite-deps.tar.bz2 ${DQLITE_ARCHIVE_PATH}

# s3 puts here are without an ACL.
# The bucket uses policies that allow GetObject to anyone,
# and PutObject to select Juju team accounts.
dqlite-deps-push: ${DQLITE_ARCHIVE_PATH}
	@echo "DQLITE: Pushing ${DQLITE_S3_ARCHIVE_PATH} to s3"
	aws s3 cp $< ${DQLITE_S3_ARCHIVE_PATH}
	@echo "DQLITE: Pushing latest-dqlite-deps-${DQLITE_BUILD_ARCH}.tar.bz2 to s3"
	aws s3 cp ${DQLITE_S3_ARCHIVE_PATH} ${DQLITE_S3_BUCKET}/latest-dqlite-deps-${DQLITE_BUILD_ARCH}.tar.bz2

dqlite-deps-pull:
	@echo "DQLITE: Cleaning up deps path"
	@mkdir -p ${DQLITE_EXTRACTED_DEPS_PATH}
	@rm -rf ${DQLITE_EXTRACTED_DEPS_ARCHIVE_PATH}
	@echo "DQLITE: Pulling latest-dqlite-deps-${DQLITE_BUILD_ARCH}.tar.bz2 from s3"
	wget -q -O- https://dqlite-static-libs.s3.amazonaws.com/latest-dqlite-deps-${DQLITE_BUILD_ARCH}.tar.bz2 - | tar xjf - -C ${DQLITE_EXTRACTED_DEPS_PATH}

dqlite-deps-check:
	@if [ ! -d ${DQLITE_EXTRACTED_DEPS_ARCHIVE_PATH} ]; then \
		$(MAKE) -s dqlite-deps-pull; \
	fi

################################################################################
# musl
################################################################################

.PHONY: musl-ensure-symlink musl-check musl-install-if-missing
musl-ensure-symlink:
	@test -d /usr/local/musl/$(d) || echo "Please run 'sudo ln -s $(s) /usr/local/musl/$(d)'"

musl-check:
	@PATH=${PATH}:/usr/local/musl/bin which musl-gcc >/dev/null || echo "Please run 'sudo make musl-install'"
	# @$(MAKE) -s musl-ensure-symlink d=include/asm s=/usr/include/${DQLITE_BUILD_MACHINE}-linux-gnu/asm
	# @$(MAKE) -s musl-ensure-symlink d=include/asm-generic s=/usr/include/asm-generic
	@$(MAKE) -s musl-ensure-symlink d=include/linux s=/usr/include/linux

musl-install:
	mkdir -p /tmp/musl
	wget -q https://musl.libc.org/releases/musl-1.2.3.tar.gz -O - | tar -xzf - -C /tmp/musl
	cd /tmp/musl/musl-1.2.3
	./configure
	make install

	cd -
	# Setup symlinks so we can access additional headers that 
	# don't ship with musl but are needed for our builds
	ln -s /usr/include/${DQLITE_BUILD_MACHINE}-linux-gnu/asm /usr/local/musl/include/asm
	ln -s /usr/include/asm-generic /usr/local/musl/include/asm-generic
	ln -s /usr/include/linux /usr/local/musl/include/linux

musl-install-if-missing:
	@PATH=${PATH}:/usr/local/musl/bin which musl-gcc >/dev/null || { echo "Installing required musl dependencies"; sudo ${MAKE} musl-install; }

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