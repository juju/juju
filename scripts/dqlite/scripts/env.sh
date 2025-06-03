#!/usr/bin/env bash

set -e

PROJECT_DIR=$(pwd)
PROJECT_BRANCH="4.0"

DEBUG_MODE=${DEBUG_MODE:-false}

is_darwin() {
	OS=$(uname -s | tr '[:upper:]' '[:lower:]')
	if [[ "${OS}" =~ ^darwin* ]]; then
		echo true
	else
		echo false
	fi
}

current_arch() {
	case $(uname -m) in
		x86_64) echo amd64 ;;
		aarch64) echo arm64 ;;
		s390x) echo s390x ;;
		ppc64le) echo ppc64le ;;
		riscv64) echo riscv64 ;;
		arm64)
			if [[ $(is_darwin) = true  ]]; then
				echo "arm64"
			else
				echo "Unsupported OS: ${OS}" && exit 1
			fi
			;;
		*) echo "Unsupported architecture $(uname -m)" && exit 1 ;;
	esac
}

check_dependencies() {
	local dep missing
	missing=""

	for dep in "$@"; do
		if ! which "$dep" >/dev/null 2>&1; then
			[[ "$missing" ]] && missing="$missing, $dep" || missing="$dep"
		fi
	done

	if [[ "$missing" ]]; then
		echo "Missing dependencies: $missing" >&2
		echo ""
		exit 1
	fi
}

CURRENT_ARCH=$(current_arch)

BUILD_IMAGE=ubuntu:22.04
BUILD_CONTAINER=lib-build-server
BUILD_MACHINE=$(uname -m)
BUILD_ARCH=$(go env GOARCH 2>/dev/null || echo "amd64")

EXTRACTED_DEPS_PATH=${PROJECT_DIR}/_deps
EXTRACTED_DEPS_ARCH_PATH=${EXTRACTED_DEPS_PATH}/dqlite-deps-${PROJECT_BRANCH}-${BUILD_ARCH}

TAG_LIBTIRPC=upstream/1.3.3
TAG_LIBNSL=v2.0.0
TAG_LIBUV=v1.46.0
TAG_LIBLZ4=v1.9.4
TAG_SQLITE=version-3.46.0
TAG_DQLITE=v1.18.1

S3_BUCKET=s3://dqlite-static-libs
S3_ARCHIVE_NAME=$(date -u +"%Y-%m-%d")-dqlite-deps-${BUILD_ARCH}.tar.bz2
S3_ARCHIVE_PATH=${S3_BUCKET}/${S3_ARCHIVE_NAME}

ARCHIVE_DEPS_PATH=${PROJECT_DIR}/_build
ARCHIVE_NAME=dqlite-deps
ARCHIVE_PATH=${ARCHIVE_DEPS_PATH}/${ARCHIVE_NAME}.tar.bz2
