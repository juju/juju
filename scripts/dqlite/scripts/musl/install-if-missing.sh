#!/bin/bash

set -e

source "$(dirname $0)/musl-install.sh"

PATH=${PATH}:${MUSL_BIN_PATH} which musl-gcc >/dev/null || { echo "Installing required musl dependencies"; install; exit 0; }

echo "musl-gcc already installed"

if [ $(is_darwin) = true ] && [ ! -f "${MUSL_LOCAL_PATH}/output/bin/musl-gcc" ]; then
    echo "Symlinking darwin musl-gcc to ${BUILD_ARCH}"
    mkdir -p ${MUSL_LOCAL_PATH}/output/bin || { echo "Failed to create ${MUSL_LOCAL_PATH}/output/bin"; exit 1; }
    BREW_PATH=$(brew --prefix)
    BREW_BIN_PATH=${BREW_PATH}/bin
    case ${BUILD_ARCH} in
		amd64) ln -s "${BREW_BIN_PATH}/x86_64-linux-musl-gcc" ${MUSL_LOCAL_PATH}/output/bin/musl-gcc || { echo "Failed to link musl-gcc"; exit 1; } ;;
		arm64) ln -s "${BREW_BIN_PATH}/aarch64-linux-musl-gcc" ${MUSL_LOCAL_PATH}/output/bin/musl-gcc || { echo "Failed to link musl-gcc"; exit 1; } ;;
		*) { echo "Unsupported arch ${BUILD_ARCH}."; exit 1; } ;;
	esac
    exit 0
fi

if [ ! -f "${MUSL_LOCAL_PATH}/output/bin/musl-gcc" ]; then
    P=$(PATH=${PATH}:${MUSL_BIN_PATH} which musl-gcc)
    echo "Symlinking ${P} to ${BUILD_ARCH}"
    mkdir -p ${MUSL_LOCAL_PATH}/output/bin || { echo "Failed to create ${MUSL_LOCAL_PATH}/output/bin"; exit 1; }
    ln -s "${P}" ${MUSL_LOCAL_PATH}/output/bin/musl-gcc || { echo "Failed to link musl-gcc"; exit 1; }
fi
