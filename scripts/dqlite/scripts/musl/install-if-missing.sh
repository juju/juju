#!/bin/bash

set -e

source "$(dirname $0)/musl-install.sh"

PATH=${PATH}:${MUSL_BIN_PATH} which musl-gcc >/dev/null || { echo "Installing required musl dependencies"; install; exit 0; }

echo "musl-gcc already installed"
