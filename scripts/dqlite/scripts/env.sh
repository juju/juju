#!/bin/bash

set -e

PROJECT_DIR=$(pwd)

current_arch() {
	case $(uname -m) in
		x86_64) echo amd64 ;;
		aarch64) echo arm64 ;;
		s390x) echo s390x ;;
		ppc64le) echo ppc64le ;;
		riscv64) echo riscv64 ;;
		*) echo "Unsupported architecture $(uname -m)" && exit 1 ;;
	esac
}

CURRENT_ARCH=$(current_arch)

BUILD_IMAGE=ubuntu:22.04
BUILD_CONTAINER=lib-build-server
BUILD_MACHINE=$(uname -m)
BUILD_ARCH=$(go env GOARCH 2>/dev/null || echo "amd64")

EXTRACTED_DEPS_PATH=${PROJECT_DIR}/_deps
EXTRACTED_DEPS_ARCH_PATH=${EXTRACTED_DEPS_PATH}/dqlite-deps-${BUILD_ARCH}

TAG_LIBTIRPC=upstream/1.3.3
TAG_LIBNSL=v2.0.0
TAG_LIBUV=v1.44.2
TAG_LIBLZ4=v1.9.4
TAG_RAFT=v0.16.0
TAG_SQLITE=version-3.40.0
TAG_DQLITE=v1.12.0
