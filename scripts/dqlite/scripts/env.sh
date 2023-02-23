#!/bin/bash

set -e

PROJECT_DIR=$(pwd)

DEBUG_MODE=${DEBUG_MODE:-false}

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
TAG_RAFT=v0.17.1
TAG_SQLITE=version-3.40.0
TAG_DQLITE=v1.14.0

S3_BUCKET=s3://dqlite-static-libs
S3_ARCHIVE_NAME=$(date -u +"%Y-%m-%d")-dqlite-deps-${BUILD_ARCH}.tar.bz2
S3_ARCHIVE_PATH=${S3_BUCKET}/${S3_ARCHIVE_NAME}

ARCHIVE_DEPS_PATH=${PROJECT_DIR}/_build
ARCHIVE_NAME=dqlite-deps
ARCHIVE_PATH=${ARCHIVE_DEPS_PATH}/${ARCHIVE_NAME}.tar.bz2
