#!/bin/bash
set -e

build() {
    set -ex

    MACHINE_TYPE=$(uname -m)
    CUSTOM_CFLAGS=""
    if [ "${MACHINE_TYPE}" = "ppc64le" ]; then
        CUSTOM_CFLAGS="-mlong-double-64"
    fi
    DQLITE_CONFIGURE_FLAGS=
    if [ "${DEBUG_MODE}" = "true" ]; then
        DQLITE_CONFIGURE_FLAGS="--enable-debug"
    fi

    # Ensure that when apt installs tzdata skips it's prompt in all contexts
    sudo ln -fs /usr/share/zoneinfo/UTC /etc/localtime

    # TODO: Make this script idempotent, so that it checks for the
    # existence of repositories, requiring only a pull and not a full clone.

    # Setup build env
    sudo apt-get update
    sudo apt-get -y -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" install \
        automake \
        libtool \
        build-essential \
        gettext \
        autopoint \
        pkg-config \
        tclsh \
        tcl \
        libsqlite3-dev \
        wget \
        git

    mkdir -p "${ARCHIVE_DEPS_PATH}"
    cd "${ARCHIVE_DEPS_PATH}"

    sudo snap install zig --classic --channel beta

    export CC="zig cc -target ${ZIG_TARGET} -D _GNU_SOURCE"
    cd ..

    # Install compile dependencies for statically linking everything:
    # --------------------------------------------------------------
    # libtirpc (required by libnsl)
    # libnsl (required by dqlite)
    # libuv (required by raft)
    # liblz4 (required by raft)
    # raft (required by dqlite)
    # sqlite3 (required by dqlite)
    # dqlite

    INCLUDE_DIR="${PWD}/include"
    mkdir -p "${INCLUDE_DIR}/sys"

    # Grab the queue.h file that does not ship with musl
    wget https://dev.midipix.org/compat/musl-compat/raw/main/f/include/sys/queue.h -O "${INCLUDE_DIR}/sys/queue.h"

    # libtirpc
    git clone https://salsa.debian.org/debian/libtirpc.git --depth 1 --branch ${TAG_LIBTIRPC}
    cd libtirpc
    chmod +x autogen.sh
    ./autogen.sh
    ./configure --disable-shared --disable-gssapi CFLAGS="-I${INCLUDE_DIR} ${CUSTOM_CFLAGS}" --target ${ZIG_TARGET} --host ${MACHINE_TYPE}
    make
    cd ../

    # libnsl
    git clone https://github.com/thkukuk/libnsl --depth 1 --branch ${TAG_LIBNSL}
    cd libnsl
    ./autogen.sh
    autoreconf -i
    autoconf
    CFLAGS="-I${PWD}/../libtirpc/tirpc ${CUSTOM_CFLAGS}" \
            LDFLAGS="-L${PWD}/../libtirpc/src" \
            TIRPC_CFLAGS="-I${PWD}/../libtirpc/tirpc" \
            TIRPC_LIBS="-L${PWD}/../libtirpc/src" \
            ./configure --disable-shared --target ${ZIG_TARGET} --host ${MACHINE_TYPE}
    make
    cd ../

    # libuv
    git clone https://github.com/libuv/libuv.git --depth 1 --branch ${TAG_LIBUV}
    cd libuv
    ./autogen.sh
    ./configure # we need the .so files as well; see note below
    make
    cd ../

    # liblz4
    git clone https://github.com/lz4/lz4.git --depth 1 --branch ${TAG_LIBLZ4}
    cd lz4
    make lib
    cd ../

    # raft
    git clone https://github.com/canonical/raft.git --depth 1 --branch ${TAG_RAFT}
    cd raft
    autoreconf -i
    CFLAGS="-I${PWD}/../libuv/include -I${PWD}/../lz4/lib ${CUSTOM_CFLAGS}" \
            LDFLAGS="-L${PWD}/../libuv/.libs -L${PWD}/../lz4/lib" \
            UV_CFLAGS="-I${PWD}/../libuv/include" \
            UV_LIBS="-L${PWD}/../libuv/.libs" \
            LZ4_CFLAGS="-I${PWD}/../lz4/lib" \
            LZ4_LIBS="-L${PWD}/../lz4/lib" \
            ./configure --disable-shared --target ${ZIG_TARGET} --host ${MACHINE_TYPE}
    make
    cd ../

    # sqlite3
    git clone https://github.com/sqlite/sqlite.git --depth 1 --branch ${TAG_SQLITE}
    cd sqlite
    ./configure --disable-shared --target ${ZIG_TARGET} --host ${MACHINE_TYPE}
    make
    cd ../

    # dqlite
    git clone https://github.com/canonical/dqlite.git --depth 1 --branch ${TAG_DQLITE}
    cd dqlite
    autoreconf -i
    CFLAGS="-I${PWD}/../raft/include -I${PWD}/../sqlite -I${PWD}/../libuv/include -I${PWD}/../lz4/lib -I/usr/local/musl/include -Wno-unused-but-set-variable -Wno-unused-parameter -Werror=implicit-function-declaration -Wno-all ${CUSTOM_CFLAGS}" \
            LDFLAGS="-L${PWD}/../raft/.libs -L${PWD}/../libuv/.libs -L${PWD}/../lz4/lib -L${PWD}/../libnsl/src" \
            RAFT_CFLAGS="-I${PWD}/../raft/include" \
            RAFT_LIBS="-L${PWD}/../raft/.libs" \
            UV_CFLAGS="-I${PWD}/../libuv/include" \
            UV_LIBS="-L${PWD}/../libuv/.libs" \
            SQLITE_CFLAGS="-I${PWD}/../sqlite" \
            ./configure --disable-shared ${DQLITE_CONFIGURE_FLAGS} --target ${ZIG_TARGET} --host ${MACHINE_TYPE}
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

    # Bill of materials
    echo "libtirpc ${TAG_LIBTIRPC}" > juju-dqlite-static-lib-deps/BOM
    echo "libnsl ${TAG_LIBNSL}" >> juju-dqlite-static-lib-deps/BOM
    echo "libuv ${TAG_LIBUV}" >> juju-dqlite-static-lib-deps/BOM
    echo "liblz4 ${TAG_LIBLZ4}" >> juju-dqlite-static-lib-deps/BOM
    echo "raft ${TAG_RAFT}" >> juju-dqlite-static-lib-deps/BOM
    echo "sqlite ${TAG_SQLITE}" >> juju-dqlite-static-lib-deps/BOM
    echo "dqlite ${TAG_DQLITE}" >> juju-dqlite-static-lib-deps/BOM

    tar cjvf ${ARCHIVE_PATH} juju-dqlite-static-lib-deps
}
