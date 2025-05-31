#!/usr/bin/env bash

set -e

source "$(dirname $0)/dqlite-install.sh"

[ -d "${EXTRACTED_DEPS_ARCH_PATH}" ] || { echo "Installing dqlite"; install; exit 0; }

echo "dqlite already installed"
