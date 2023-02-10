#!/bin/bash

set -e

source "$(dirname $0)/musl-install.sh"

PATH=${PATH}:${MUSL_BIN_PATH} which musl-gcc >/dev/null || { echo "Installing required musl dependencies"; install; exit 0; }

echo "musl-gcc already installed"

if [ ! -f "${MUSL_LOCAL_PATH}/output/bin/musl-gcc" ]; then
    P=$(PATH=${PATH}:${MUSL_BIN_PATH} which musl-gcc)
    echo "Symlinking ${P}"
    mkdir -p ${MUSL_LOCAL_PATH}/output/bin || { echo "Failed to create ${MUSL_LOCAL_PATH}/output/bin"; exit 1; }
    ln -s "${P}" ${MUSL_LOCAL_PATH}/output/bin/musl-gcc || { echo "Failed to link musl-gcc"; exit 1; }
fi
