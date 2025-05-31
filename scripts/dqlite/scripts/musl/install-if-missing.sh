#!/usr/bin/env bash

set -e

source "$(dirname $0)/musl-install.sh"

PATH=${MUSL_BIN_PATH}:${PATH} which musl-gcc >/dev/null || { echo "Installing required musl dependencies"; install; exit 0; }

echo "musl-gcc already installed"

if [ $(is_darwin) = true ] && [ ! -f "${MUSL_LOCAL_PATH}/output/bin/musl-gcc" ]; then
    post_musl_install_cross_darwin
    exit 0
fi

if [ ! -f "${MUSL_LOCAL_PATH}/output/bin/musl-gcc" ]; then
    P=$(PATH=${MUSL_BIN_PATH}:${PATH} which musl-gcc)
    echo "Symlinking ${P} to ${BUILD_ARCH}"
    mkdir -p ${MUSL_LOCAL_PATH}/output/bin || { echo "Failed to create ${MUSL_LOCAL_PATH}/output/bin"; exit 1; }
    ln -s "${P}" ${MUSL_LOCAL_PATH}/output/bin/musl-gcc || { echo "Failed to link musl-gcc"; exit 1; }
fi
