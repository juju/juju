#!/bin/bash

set -e

PROJECT_DIR=$(pwd)

current_arch() {
	case $(uname -m) in
		x86_64) echo amd64 ;;
		aarch64) echo arm64 ;;
		s390x) echo s390x ;;
		powerpc64le) echo ppc64le ;;
		riscv64) echo riscv64 ;;
		*) exit 1 ;;
	esac
}

CURRENT_ARCH=$(current_arch)

BUILD_IMAGE=ubuntu:22.04
BUILD_CONTAINER=lib-build-server
BUILD_MACHINE=$(uname -m)
BUILD_ARCH=$(go env GOARCH)
