#!/bin/bash

set -e

source "$(dirname $0)/../env.sh"

MUSL_VERSION="1.2.3"
MUSL_PRECOMPILED=${MUSL_PRECOMPILED:-"1"}
MUSL_CROSS_COMPILE=${MUSL_CROSS_COMPILE:-"1"}

MUSL_LOCAL_PLACEMENT=${MUSL_LOCAL_PLACEMENT:-"system"}

MUSL_LOCAL_PATH=${EXTRACTED_DEPS_PATH}/musl-${BUILD_ARCH}
MUSL_SYSTEM_PATH=/usr/local/musl

if [ "${MUSL_LOCAL_PLACEMENT}" = "local" ] || [ "${MUSL_CROSS_COMPILE}" = "1" ]; then
    MUSL_PATH=${MUSL_LOCAL_PATH}
    MUSL_BIN_PATH=${MUSL_PATH}/output/bin
else
    MUSL_PATH=${MUSL_SYSTEM_PATH}
    MUSL_BIN_PATH=${MUSL_PATH}/bin
fi

musl_install_system() {
    sudo ./configure || { echo "Failed to configure musl"; exit 1; }
    sudo make install || { echo "Failed to install musl"; exit 1; }

    LOCAL_PATH=${EXTRACTED_DEPS_PATH}/musl-${BUILD_ARCH}/output/bin

    mkdir -p ${LOCAL_PATH} || { echo "Failed to create ${MUSL_BIN_PATH}"; exit 1; }
    sudo ln -s ${MUSL_BIN_PATH}/musl-gcc ${LOCAL_PATH}/musl-gcc || { echo "Failed to link musl-gcc"; exit 1; }

    sudo ln -s /usr/include/${BUILD_MACHINE}-linux-gnu/asm ${MUSL_PATH}/include/asm || { echo "Failed to link ${BUILD_MACHINE}-linux-gnu/asm headers"; exit 1; }
    sudo ln -s /usr/include/asm-generic ${MUSL_PATH}/include/asm-generic || { echo "Failed to link asm-generic headers"; exit 1; }
    sudo ln -s /usr/include/linux ${MUSL_PATH}/include/linux || { echo "Failed to link linux header"; exit 1; } 
}

musl_install_local() {
    ./configure --prefix=${MUSL_PATH} || { echo "Failed to configure musl"; exit 1; }
    make install || { echo "Failed to install musl"; exit 1; }

    mkdir -p ${MUSL_BIN_PATH} || { echo "Failed to create ${MUSL_BIN_PATH}"; exit 1; }
    ln -s ${MUSL_PATH}/bin/musl-gcc ${MUSL_BIN_PATH}/musl-gcc || { echo "Failed to link musl-gcc"; exit 1; }

    cd ${PROJECT_DIR}
    ln -s /usr/include/${BUILD_MACHINE}-linux-gnu/asm ${MUSL_PATH}/include/asm || { echo "Failed to link ${BUILD_MACHINE}-linux-gnu/asm headers"; exit 1; }
    ln -s /usr/include/asm-generic ${MUSL_PATH}/include/asm-generic || { echo "Failed to link asm-generic headers"; exit 1; }
    ln -s /usr/include/linux ${MUSL_PATH}/include/linux || { echo "Failed to link linux header"; exit 1; }
}

musl_install() {
    TMP_DIR=$(mktemp -d)
    wget -q https://musl.libc.org/releases/musl-${MUSL_VERSION}.tar.gz -O - | tar -xzf - -C ${TMP_DIR}
    cd ${TMP_DIR}/musl-${MUSL_VERSION}

    if [ "${MUSL_LOCAL_PLACEMENT}" = "local" ]; then
        echo "Installing local musl"
        musl_install_local
    else
        echo "Installing system musl"
        musl_install_system
    fi
}

musl_install_cross_arch() {
    mkdir -p ${MUSL_PATH} || { exit 1; }
    git clone https://github.com/richfelker/musl-cross-make.git ${MUSL_PATH}
    cd ${MUSL_PATH}

    mkdir -p ${MUSL_PATH}/build

    case "${BUILD_ARCH}" in
        amd64)   echo "TARGET=x86_64-linux-musl" >> config.mak ;;
        arm64)   echo "TARGET=aarch64-linux-musl" >> config.mak ;;
        s390x)   echo "TARGET=s390x-linux-musl" >> config.mak ;;
        ppc64le) echo "TARGET=powerpc64le-linux-musl" >> config.mak ;;
        riscv64) echo "TARGET=riscv64-linux-musl" >> config.mak ;;
        *)
            echo "Unsupported architecture ${BUILD_ARCH}"
            exit 1
            ;;
    esac

    echo "OUTPUT=${MUSL_PATH}/output" >> config.mak
    echo "COMMON_CONFIG += CFLAGS=\"-g0 -Os\" CXXFLAGS=\"-g0 -Os\" LDFLAGS=\"-s\"" >> config.mak

    echo "Building musl-${BUILD_ARCH}"
    make install || { exit 1; }

    echo "Linking musl-${BUILD_ARCH} to musl-gcc"
    cd ${MUSL_PATH}/output/bin

    case "${BUILD_ARCH}" in
        amd64) ln -s x86_64-linux-musl-gcc musl-gcc ;;
        arm64) ln -s aarch64-linux-musl-gcc musl-gcc ;;
        s390x) ln -s s390x-linux-musl-gcc musl-gcc ;;
        ppc64le) ln -s powerpc64le-linux-musl-gcc musl-gcc ;;
        riscv64) ln -s riscv64-linux-musl-gcc musl-gcc ;;
        *)
            echo "Unsupported architecture ${BUILD_ARCH}"
            exit 1
            ;;
    esac
}

sha() {
    case ${BUILD_ARCH} in
        amd64) echo "c19e7337cd28232b44b19db7da68089dd1b957a474440046c113e507b5af0290" ;;
        *) echo "" ;;
    esac
}

musl_install_precompiled_cross_arch() {
    mkdir -p ${EXTRACTED_DEPS_PATH} || { exit 1; }
    cd ${EXTRACTED_DEPS_PATH}

    SHA=$(sha)
    if [ "${SHA}" = "" ]; then
        echo "No precompiled musl for ${BUILD_ARCH} falling back to building"
        musl_install_cross_arch
        exit 0
    fi

    echo "Downloading precompiled musl for ${BUILD_ARCH}"
    
    FILE="$(mktemp -d)/musl-${BUILD_ARCH}.tar.bz2"

    name=${SHA}.tar.bz2
    echo " + Retrieving ${name}"
    curl --fail -o ${FILE} -s https://dqlite-static-libs.s3.amazonaws.com/musl/${name} || {
			echo " + Failed to retrieve ${name}";
			rm -f ${FILE} || true;
			exit 1;
		}

    SUM=$(sha256sum ${FILE} | awk '{print $1}')
    if [ "${SUM}" != ${SHA} ]; then
        echo "sha256sum mismatch (${SUM}, expected $(sha))"
        exit 1
    fi

    echo " + Extracting ${FILE}"
    tar -xjf ${FILE} -C ${EXTRACTED_DEPS_PATH} || { echo "Failed to extract musl"; exit 1; }
}

musl_install_cross_darwin() {
    echo "Installing musl-cross for darwin"
    brew --version >/dev/null || { echo "homebrew not installed"; exit 1; }
    brew install -q filosottile/musl-cross/musl-cross --with-aarch64 --with-x86_64 || { echo "Failed to install musl-cross"; exit 1; }
}

install() {
    if [[ $(is_darwin) = true ]]; then
        musl_install_cross_darwin && exit 0
    fi
    if [ "${MUSL_PRECOMPILED}" = "1" ]; then
        echo "Installing precompiled musl"
        musl_install_precompiled_cross_arch
        exit 0
    fi
    if [ "${MUSL_CROSS_COMPILE}" = "1" ]; then
        echo "Installing cross-arch musl"
        musl_install_cross_arch
        exit 0
    fi

    musl_install
    exit 0
}