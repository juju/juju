SHELL = bash
.ONESHELL:

JUJU_PROJECT_PATH=$(GOPATH)/src/github.com/juju/juju

DQLITE_BUILD_IMAGE=ubuntu:18.04
DQLITE_BUILD_CONTAINER=lib-build-server

DQLITE_ARCHIVE_DEPS_PATH=${JUJU_PROJECT_PATH}/scripts/dqlite
DQLITE_ARCHIVE_NAME=dqlite-deps
DQLITE_ARCHIVE_PATH=${DQLITE_ARCHIVE_DEPS_PATH}/${DQLITE_ARCHIVE_NAME}.tar.bz2

DQLITE_S3_BUCKET=s3://dqlite-static-libs
DQLITE_S3_ARCHIVE_NAME=$(shell date -u +"%Y-%m-%d")-dqlite-deps-$(shell uname -m).tar.bz2
DQLITE_S3_ARCHIVE_PATH=${DQLITE_S3_BUCKET}/${DQLITE_S3_ARCHIVE_NAME}

DQLITE_EXTRACTED_DEPS_PATH=${JUJU_PROJECT_PATH}/_deps
DQLITE_EXTRACTED_DEPS_ARCHIVE_PATH=${DQLITE_EXTRACTED_DEPS_PATH}/juju-dqlite-static-lib-deps

.PHONY: install-deps-on-controller build-and-install-jujud

${DQLITE_ARCHIVE_PATH}:
	@lxc delete -f ${DQLITE_BUILD_CONTAINER} &>/dev/null || true

	@echo "DQLITE: Creating build box using ${DQLITE_BUILD_IMAGE}"
	@lxc launch ubuntu:18.04 ${DQLITE_BUILD_CONTAINER}

	@echo "DQLITE: Waiting for ${DQLITE_BUILD_CONTAINER} to become ready..."
	@lxc exec ${DQLITE_BUILD_CONTAINER} -- bash -c 'while [ "$$(systemctl is-system-running 2>/dev/null)" != "running" ] && [ "$$(systemctl is-system-running 2>/dev/null)" != "degraded" ]; do :; done'

	@echo "DQLITE: Preparing provisioning script: /tmp/setup-build-farm.sh"
	@cat <<- 'EOF' >/tmp/setup-build-farm.sh
		#!/bin/bash
		set -e

		# TODO: Make this script idempotent, so that it checks for the
		# existence of repositories, requiring only a pull and not a full clone.

		# Setup build env
		apt-get update
		apt-get -y -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" install \
			gcc automake libtool make gettext autopoint pkg-config tclsh tcl libsqlite3-dev

		mkdir build
		cd build

		# Checkout and build musl. We will use this to avoid depending
		# on the hosts libc.
		#
		# TODO: investigate zig-gcc as an alternative.
		wget https://musl.libc.org/releases/musl-1.2.2.tar.gz
		tar xf musl-1.2.2.tar.gz
		cd musl-1.2.2
		./configure
		make install
	
		export PATH=$${PATH}:/usr/local/musl/bin
		export CC=musl-gcc
		cd ..

		# Setup symlinks so we can access additional headers that 
		# don't ship with muscl but are needed for our builds
		ln -s /usr/include/x86_64-linux-gnu/asm /usr/local/musl/include/asm
		ln -s /usr/include/asm-generic /usr/local/musl/include/asm-generic
		ln -s /usr/include/linux /usr/local/musl/include/linux

		# Grab the queue.h file that does not ship with musl
		wget https://dev.midipix.org/compat/musl-compat/raw/main/f/include/sys/queue.h -O /usr/local/musl/include/sys/queue.h

		# Install compile dependencies for statically linking everything:
		# --------------------------------------------------------------
		# libtirpc (required by libnsl)
		# libnsl (required by dqlite)
		# libuv (required by raft)
		# liblz4 (required by raft)
		# raft (required by dqlite)
		# sqlite3 (required by dqlite)
		# dqlite
		
		# libtirpc
		git clone git://linux-nfs.org/~steved/libtirpc
		cd libtirpc
		chmod +x autogen.sh
		./autogen.sh
		./configure --disable-shared --disable-gssapi
		make
		cd ../

		# libnsl
		git clone https://github.com/thkukuk/libnsl
		cd libnsl
		./autogen.sh
		autoreconf -i
		autoconf
		CFLAGS="-I$${PWD}/../libtirpc/tirpc" \
		       LDFLAGS="-L$${PWD}/../libtirpc/src" \
		       TIRPC_CFLAGS="-I$${PWD}/../libtirpc/tirpc" \
		       TIRPC_LIBS="-L$${PWD}/../libtirpc/src" \
		       ./configure --disable-shared
		make
		cd ../

		# libuv
		git clone https://github.com/libuv/libuv.git --depth 1
		cd libuv
		./autogen.sh
		./configure # we need the .so files as well; see note below
		make
		cd ../

		# liblz4
		git clone https://github.com/lz4/lz4.git --depth 1
		cd lz4
		make lib
		cd ../

		# raft
		git clone https://github.com/canonical/raft.git --depth 1
		cd raft
		autoreconf -i
		CFLAGS="-I$${PWD}/../libuv/include -I$${PWD}/../lz4/lib" \
		       LDFLAGS="-L$${PWD}/../libuv/.libs -L$${PWD}/../lz4/lib" \
		       UV_CFLAGS="-I$${PWD}/../libuv/include" \
		       UV_LIBS="-L$${PWD}/../libuv/.libs" \
		       LZ4_CFLAGS="-I$${PWD}/../lz4/lib" \
		       LZ4_LIBS="-L$${PWD}/../lz4/lib" \
		       ./configure --disable-shared
		make
		cd ../

		# sqlite3
		git clone https://github.com/sqlite/sqlite.git --depth 1
		cd sqlite
		./configure --disable-shared
		make
		cd ../
	
		# dqlite
		git clone https://github.com/canonical/dqlite.git  --depth 1
		cd dqlite
		autoreconf -i
		CFLAGS="-I$${PWD}/../raft/include -I$${PWD}/../sqlite -I$${PWD}/../libuv/include -I$${PWD}/../lz4/lib -I/usr/local/musl/include -Werror=implicit-function-declaration" \
		       LDFLAGS="-L$${PWD}/../raft/.libs -L$${PWD}/../libuv/.libs -L$${PWD}/../lz4/lib -L$${PWD}/../libnsl/src" \
		       RAFT_CFLAGS="-I$${PWD}/../raft/include" \
		       RAFT_LIBS="-L$${PWD}/../raft/.libs" \
		       UV_CFLAGS="-I$${PWD}/../libuv/include" \
		       UV_LIBS="-L$${PWD}/../libuv/.libs" \
		       SQLITE_CFLAGS="-I$${PWD}/../sqlite" \
		       ./configure --disable-shared
		make
		cd ../
		
		rm -Rf juju-dqlite-static-lib-deps
		mkdir juju-dqlite-static-lib-deps

		# Collect .a files
		# NOTE: for some strange reason we *also* require the libuv and
		# liblz4 .so files for the final juju link step even though the
		# resulting artifact is statically linked.
		cp libuv/.libs/* juju-dqlite-static-lib-deps/
		cp lz4/lib/*.a juju-dqlite-static-lib-deps/
		cp lz4/lib/*.so* juju-dqlite-static-lib-deps/
		cp raft/.libs/*.a juju-dqlite-static-lib-deps/
		cp sqlite/.libs/*.a juju-dqlite-static-lib-deps/
		cp dqlite/.libs/*.a juju-dqlite-static-lib-deps/

		# Collect required headers
		mkdir juju-dqlite-static-lib-deps/include
		cp -r raft/include/* juju-dqlite-static-lib-deps/include
		cp -r sqlite/*.h juju-dqlite-static-lib-deps/include
		cp -r dqlite/include/* juju-dqlite-static-lib-deps/include

		tar cjvf juju-dqlite-static-lib-deps.tar.bz2 juju-dqlite-static-lib-deps
	EOF

	@echo "DQLITE: Pushing provisioning script to ${DQLITE_BUILD_CONTAINER}"
	@lxc file push /tmp/setup-build-farm.sh ${DQLITE_BUILD_CONTAINER}/root/setup-build-farm.sh

	@echo "DQLITE: Executing provisioning script"
	@lxc exec -t ${DQLITE_BUILD_CONTAINER} bash /root/setup-build-farm.sh

	@echo "DQLITE: Pulling lib archive"
	@lxc file pull ${DQLITE_BUILD_CONTAINER}/root/build/juju-dqlite-static-lib-deps.tar.bz2 $@
	@lxc delete -f $(DQLITE_BUILD_CONTAINER)

dqlite-deps-push: ${DQLITE_ARCHIVE_PATH}
	@echo "DQLITE: Pushing ${DQLITE_S3_ARCHIVE_PATH} to s3"
	aws s3 cp --acl public-read $< ${DQLITE_S3_ARCHIVE_PATH}
	@echo "DQLITE: Pushing latest-juju-dqlite-static-lib-deps-$(shell uname -m).tar.bz2 to s3"
	aws s3 cp --acl public-read ${DQLITE_S3_ARCHIVE_PATH} ${DQLITE_S3_BUCKET}/latest-dqlite-deps-$(shell uname -m).tar.bz2

dqlite-deps-pull:
	@echo "DQLITE: Cleaning up deps path"
	@mkdir -p ${DQLITE_EXTRACTED_DEPS_PATH}
	@rm -rf ${DQLITE_EXTRACTED_DEPS_ARCHIVE_PATH}
	@echo "DQLITE: Pulling latest-juju-dqlite-static-lib-deps-$(shell uname -m).tar.bz2 from s3"
	aws s3 cp s3://dqlite-static-libs/latest-juju-dqlite-static-lib-deps-$(shell uname -m).tar.bz2 - | tar xjf - -C ${DQLITE_EXTRACTED_DEPS_PATH}

dqlite-deps-check:
	@if [ ! -d ${DQLITE_EXTRACTED_DEPS_ARCHIVE_PATH} ]; then \
		$(MAKE) -s dqlite-deps-pull; \
	fi

################################################################################
# musl
################################################################################

.PHONY: musl-ensure-symlink musl-check
musl-ensure-symlink:
	@test -d /usr/local/musl/$(d) || echo "Please run 'sudo ln -s $(s) /usr/local/musl/$(d)'"

musl-check:
	@PATH=${PATH}:/usr/local/musl/bin which musl-gcc >/dev/null || echo "Please run 'sudo make musl-install'"
	# @$(MAKE) -s musl-ensure-symlink d=include/asm s=/usr/include/$(shell uname -m)-linux-gnu/asm
	# @$(MAKE) -s musl-ensure-symlink d=include/asm-generic s=/usr/include/asm-generic
	@$(MAKE) -s musl-ensure-symlink d=include/linux s=/usr/include/linux

musl-install:
	wget https://musl.libc.org/releases/musl-1.2.2.tar.gz -O- | tar xf - -C /tmp/musls/musl-1.2.2.tar.gz
	cd /tmp/musls/musl-1.2.2.tar.gz
	./configure
	make install

	cd ..
	# Setup symlinks so we can access additional headers that 
	# don't ship with muscl but are needed for our builds
	# ln -s /usr/include/$(shell uname -m)-linux-gnu/asm /usr/local/musl/include/asm
	# ln -s /usr/include/asm-generic /usr/local/musl/include/asm-generic
	ln -s /usr/include/linux /usr/local/musl/include/linux

################################################################################
# REPL
#
# Accessing the dqlite repl on a given controller for debugging purposes.
################################################################################

JUJU_CONTROLLER_BOX ?= $(shell juju status -m controller | grep juju- | head -1 | awk '{print $$4}')

juju-dqlite-repl-deps-on-controller:
	@lxc exec -t ${JUJU_CONTROLLER_BOX} -- bash -c 'which rlwrap &>/dev/null || apt-get -y -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" install rlwrap'
	@lxc exec -t ${JUJU_CONTROLLER_BOX} -- bash -c 'which socat &>/dev/null || apt-get -y -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" install socat'

juju-dqlite-repl: juju-dqlite-repl-deps-on-controller
	@echo "[+] Connecting to REPL interface on controller: ${JUJU_CONTROLLER_BOX}"
	@lxc exec -t ${JUJU_CONTROLLER_BOX} -- bash -c 'rlwrap -H /root/.dqlite_repl.history socat - /var/lib/juju/dqlite/juju.sock'
