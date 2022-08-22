#!/usr/bin/env bash

TAG_LIBTIRPC=upstream/1.3.3
TAG_LIBNSL=v2.0.0
TAG_LIBUV=v1.44.2
TAG_LIBLZ4=v1.9.4
TAG_RAFT=v0.16.0
TAG_SQLITE=version-3.40.0
TAG_DQLITE=v1.12.0

go_osarch_to_zig_target () {
	case $1 in 
		linux/arm64)
			echo -n "aarch64-linux-musl"
			;;
		linux/amd64)
			echo -n "x86_64-linux-musl"
			;;
		linux/ppc64le)
			echo -n "powerpc64le-linux-musl"
			;;
		linux/ppc64el)
			echo -n "powerpc64le-linux-musl"
			;;
		linux/s390x)
			echo -n "s390x-linux-musl"
			;;
		*)
			echo -n ""
			;;
	esac
}

		set -e

		if [ -z "$1" ]; then
			echo "you must specify the os/arch tuple to build dqlite for when running this script"
			exit 1
		fi

		set -u

		OS=$(echo $1 | cut -f 1 -d /)
		ARCH=$(echo $1 | cut -f 2 -d /)
		echo "### building dqlite artefacts for OS \"${OS}\" and ARCH \"${ARCH}\""

		ZIG_TARGET=$(go_osarch_to_zig_target $1)
		if [ -z $ZIG_TARGET ]; then
			echo "unsupported os/arch pair $1"
			exit 1
		fi

		BUILD_DIR=${BUILD_DIR:-$(pwd)/build_${OS}_${ARCH}}
		echo "### making build dir ${BUILD_DIR}"
		mkdir -p "${BUILD_DIR}"
		cd "$BUILD_DIR"

		export CC="zig cc -target $ZIG_TARGET -D _GNU_SOURCE"
		echo "### setting CC to $CC"

		INCLUDE_DIR="$(pwd)/include"
		mkdir -p "$INCLUDE_DIR"

		# Grab the queue.h file that does not ship with musl
		echo "### grabbing include header <sys/queue.h>"
		if [ ! -f "${INCLUDE_DIR}/sys/queue.h" ]; then
			echo "### fetching queue.h"
			mkdir -p "${INCLUDE_DIR}/sys"
			wget https://dev.midipix.org/compat/musl-compat/raw/main/f/include/sys/queue.h -O "${INCLUDE_DIR}/sys/queue.h"
		fi

		# libtirpc
		echo "### compiling libtirpc"
		LIBTIRPC_DIR="$(pwd)/libtirpc"
		if [ ! -d $LIBTIRPC_DIR ]; then
			git clone https://salsa.debian.org/debian/libtirpc.git --depth 1 --branch ${TAG_LIBTIRPC}
		fi
		cd $LIBTIRPC_DIR
		chmod +x autogen.sh
		./autogen.sh
		CFLAGS="-I${INCLUDE_DIR}" \
			./configure --disable-shared --disable-gssapi --target $ZIG_TARGET --host $(uname -m)
		make
		cd ..

		## libnsl
		echo "### compiling libnsl"
		LIBNSL_DIR="$(pwd)/libnsl"
		if [ ! -d $LIBNSL_DIR ]; then
			git clone 'https://github.com/thkukuk/libnsl.git' --depth 1 --branch ${TAG_LIBNSL}
		fi
		cd $LIBNSL_DIR
		./autogen.sh
		autoreconf -i
		autoconf
		CFLAGS="-I${LIBTIRPC_DIR}/tirpc" \
		       LDFLAGS="-L${LIBTIRPC_DIR}/src" \
		       TIRPC_CFLAGS="-I${LIBTIRPC_DIR}/tirpc" \
		       TIRPC_LIBS="-L${LIBTIRPC_DIR}/src" \
		       ./configure --disable-shared --target $ZIG_TARGET --host $(uname -m)
		make
		cd ..

		## libuv
		echo "### compiling libuv"
		LIBUV_DIR="$(pwd)/libuv"
		if [ ! -d $LIBUV_DIR ]; then
			git clone https://github.com/libuv/libuv.git --depth 1 --branch ${TAG_LIBUV}
		fi
		cd $LIBUV_DIR
		./autogen.sh
		./configure --disable-shared --target $ZIG_TARGET --host $(uname -m)
		make
		cd ..

		# liblz4
		echo "### compiling lz4"
		LZ4_DIR="$(pwd)/lz4"
		if [ ! -d $LZ4_DIR ]; then
			git clone https://github.com/lz4/lz4.git --depth 1 --branch ${TAG_LIBLZ4}
		fi
		cd $LZ4_DIR
		make lib
		cd ..

		# raft
		echo "### compiling raft"
		RAFT_DIR="$(pwd)/raft"
		if [ ! -d $RAFT_DIR ]; then
			git clone https://github.com/canonical/raft.git --depth 1 --branch ${TAG_RAFT}
		fi
		cd $RAFT_DIR
		autoreconf -i
		CFLAGS="-I${LIBUV_DIR}/include -I${LZ4_DIR}/lib" \
		       LDFLAGS="-L${LIBUV_DIR}/.libs -L${LZ4_DIR}/lib" \
		       UV_CFLAGS="-I${LIBUV_DIR}/include" \
		       UV_LIBS="-L${LIBUV_DIR}/.libs" \
		       LZ4_CFLAGS="-I${LZ4_DIR}/lib" \
		       LZ4_LIBS="-L${LZ4_DIR}/lib" \
		       ./configure --disable-shared --target $ZIG_TARGET --host $(uname -m)
		make
		cd ..

		# sqlite3
		echo "### compiling sqlite3"
		SQLITE_DIR="$(pwd)/sqlite"
		if [ ! -d $SQLITE_DIR ]; then
			git clone https://github.com/sqlite/sqlite.git --depth 1 --branch ${TAG_SQLITE}
		fi
		cd $SQLITE_DIR
		./configure --disable-shared --target $ZIG_TARGET --host $(uname -m)
		make
		cd ..

		# dqlite
		echo "### compiling dqlite"
		DQLITE_DIR="$(pwd)/dqlite"
		if [ ! -d $DQLITE_DIR ]; then
			git clone https://github.com/canonical/dqlite.git --depth 1 --branch ${TAG_DQLITE}
		fi
		cd $DQLITE_DIR
		autoreconf -i
		CFLAGS="-I${RAFT_DIR}/include -I${SQLITE_DIR} -I${LIBUV_DIR}/include -I${LZ4_DIR}/lib -DNDEBUG -Wno-unused-but-set-variable -Wno-unused-parameter -Wno-all"\
		       LDFLAGS="-L${RAFT_DIR}/.libs -L${LIBUV_DIR}/.libs -L${LZ4_DIR}/lib -L${LIBNSL_DIR}/src" \
		       RAFT_CFLAGS="-I${RAFT_DIR}/include" \
		       RAFT_LIBS="-L${RAFT_DIR}/.libs" \
		       UV_CFLAGS="-I${LIBUV_DIR}/include" \
		       UV_LIBS="-L${LIBUV_DIR}/.libs" \
		       SQLITE_CFLAGS="-I${SQLITE_DIR}" \
		       ./configure --disable-shared --target $ZIG_TARGET --host $(uname -m)
		make
		cd ..

		# Time to gather up all the compiled targets and make the artefacts
		DEPS_DIR=juju-dqlite-static-lib-deps
		DEPS_LIB_DIR=${DEPS_DIR}/lib
		DEPS_INCLUDE_DIR=${DEPS_DIR}/include
		rm -Rf $DEPS_DIR
		mkdir -p $DEPS_DIR
		mkdir -p $DEPS_LIB_DIR
		mkdir -p $DEPS_INCLUDE_DIR

		cp ${LIBUV_DIR}/.libs/libuv.a $DEPS_LIB_DIR
		cp ${LZ4_DIR}/lib/liblz4.a $DEPS_LIB_DIR
		cp ${RAFT_DIR}/.libs/libraft.a $DEPS_LIB_DIR
		cp ${SQLITE_DIR}/.libs/libsqlite3.a $DEPS_LIB_DIR
		cp ${DQLITE_DIR}/.libs/libdqlite.a $DEPS_LIB_DIR

		cp -r ${RAFT_DIR}/include/* $DEPS_INCLUDE_DIR
		cp -r ${SQLITE_DIR}/*.h $DEPS_INCLUDE_DIR
		cp -r ${DQLITE_DIR}/include/* $DEPS_INCLUDE_DIR

		tar cjvf juju-dqlite-static-lib-deps.tar.bz2 $DEPS_DIR
