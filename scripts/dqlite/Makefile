SHELL = bash
.ONESHELL:

BOX=lib-build-server
BOX_IMAGE=ubuntu:18.04
JUJU_CONTROLLER_BOX ?= $(shell juju status -m controller | grep juju- | head -1 | awk '{print $$4}')

.PHONY: bootstrap install-deps-on-controller build-and-install-jujud

bootstrap:
	@echo "Starting ${BOX_IMAGE}"
	@lxc launch ubuntu:18.04 ${BOX}

	@echo "Waiting for ${BOX} to become ready (get IP etc.)"
	@lxc exec ${BOX} -- bash -c 'while [ "$$(systemctl is-system-running 2>/dev/null)" != "running" ] && [ "$$(systemctl is-system-running 2>/dev/null)" != "degraded" ]; do :; done'

	@echo "Preparing provisioning script: /tmp/setup-build-farm.sh"
	@cat <<- 'EOF' >/tmp/setup-build-farm.sh
		#!/bin/bash
		set -e

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

		# Collect .a files
		rm -Rf juju-dqlite-static-lib-deps
		mkdir juju-dqlite-static-lib-deps
		# NOTE: for some strange reason we *also* require the libuv and
		# liblz4 .so files for the final juju link step even though the
		# resulting artifact is statically linked.
		cp libuv/.libs/* juju-dqlite-static-lib-deps/
		cp lz4/lib/*.a juju-dqlite-static-lib-deps/
		cp lz4/lib/*.so* juju-dqlite-static-lib-deps/
		cp raft/.libs/*.a juju-dqlite-static-lib-deps/
		cp sqlite/.libs/*.a juju-dqlite-static-lib-deps/
		cp dqlite/.libs/*.a juju-dqlite-static-lib-deps/
		tar cjvf juju-dqlite-static-lib-deps.tar.bz2 juju-dqlite-static-lib-deps
	EOF

	@echo "Pushing provisioning script to ${BOX}"
	@lxc file push /tmp/setup-build-farm.sh ${BOX}/root/setup-build-farm.sh

	@echo "Executing provisioning script"
	@lxc exec -t ${BOX} bash /root/setup-build-farm.sh

	@echo "Pulling lib archive"
	@lxc file pull ${BOX}/root/build/juju-dqlite-static-lib-deps.tar.bz2 juju-dqlite-static-lib-deps.tar.bz2

install-deps-on-controller:
	@echo "[+] Installing dqlite dependencies to controller: ${JUJU_CONTROLLER_BOX}"
	lxc exec -t ${JUJU_CONTROLLER_BOX} -- add-apt-repository -y ppa:dqlite/dev
	lxc exec -t ${JUJU_CONTROLLER_BOX} -- apt-get update
	lxc exec -t ${JUJU_CONTROLLER_BOX} -- apt-get -y -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" install libuv1-dev libsqlite3-dev libdqlite-dev rlwrap socat

stop-jujud:
	@echo
	@echo "[+] Stopping jujud-machine service on controller: ${JUJU_CONTROLLER_BOX}"
	@lxc exec ${JUJU_CONTROLLER_BOX} -- bash -c 'systemctl stop $$(ls /etc/systemd/system/jujud-mach*.service | cut -d"/" -f5)'

build-and-install-jujud:
	@echo "[+] Building jujud on ${BOX}"
	@lxc exec -t ${BOX} -- bash /root/build-juju.sh

	@echo
	@echo "[+] Stopping jujud-machine service on controller: ${JUJU_CONTROLLER_BOX}"
	@lxc exec ${JUJU_CONTROLLER_BOX} -- bash -c 'systemctl stop $$(ls /etc/systemd/system/jujud-mach*.service | cut -d"/" -f5)'

	@echo "[+] Pushing jujud to controller: ${JUJU_CONTROLLER_BOX}"
	@lxc file pull ${BOX}/tmp/jujubuild/linux_amd64/bin/jujud /tmp/jujud
	@lxc exec ${JUJU_CONTROLLER_BOX} -- bash -c 'rm -rf /var/lib/juju/tools/$$(ls /var/lib/juju/tools | grep machine-)/jujud'
	lxc file push /tmp/jujud ${JUJU_CONTROLLER_BOX}/var/lib/juju/tools/`lxc exec ${JUJU_CONTROLLER_BOX} -- bash -c 'ls /var/lib/juju/tools | grep machine-'`/jujud

	@echo "[+] Starting jujud-machine service on controller: ${JUJU_CONTROLLER_BOX}"
	@lxc exec ${JUJU_CONTROLLER_BOX} -- bash -c 'systemctl start $$(ls /etc/systemd/system/jujud-mach*.service | cut -d"/" -f5)'

dqlite-repl:
	@echo "[+] Connecting to REPL interface on controller: ${JUJU_CONTROLLER_BOX}"
	@lxc exec -t ${JUJU_CONTROLLER_BOX} -- rlwrap -H /root/.dqlite_repl.history socat - /var/lib/juju/dqlite/juju.sock