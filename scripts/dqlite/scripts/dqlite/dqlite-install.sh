#!/usr/bin/env bash

set -e

source "$(dirname $0)/../env.sh"

check_dependencies sha256sum

sha() {
	case ${BUILD_ARCH} in
	amd64) echo "45c9bbc1291e3ae3a1254e8098fda0e1a4d9b0a64178093e4e253e6ac75caa3b" ;;
	arm64) echo "e4c1e2ca250a80bc389930495bafa78dd45bd411eaca05b79bf72839c6050904" ;;
	*) {
		echo "Unsupported arch ${BUILD_ARCH}."
		exit 1
	} ;;
	esac
}

FILE="$(mktemp -d)/latest-dqlite-deps-${BUILD_ARCH}.tar.bz2"

retrieve() {
	local filenames sha

	sha=${1}

	filenames=("${sha}.tar.bz2")
	for name in "${filenames[@]}"; do
		echo "Retrieving ${name}"
		curl --fail -o ${FILE} -s https://dqlite-static-libs.s3.amazonaws.com/${name} && return || {
			echo " + Failed to retrieve ${name}"
			rm -f ${FILE} || true
			true
		}
	done
}

install() {
	mkdir -p ${EXTRACTED_DEPS_PATH}
	SHA=$(sha)
	retrieve ${SHA}
	if [ ! -f ${FILE} ]; then
		echo "Failed to retrieve dqlite static libs"
		exit 1
	fi

	SUM=$(sha256sum ${FILE} | awk '{print $1}')
	if [ "${SUM}" != ${SHA} ]; then
		echo "sha256sum mismatch (${SUM}, expected $(sha))"
		exit 1
	fi

	echo "${EXTRACTED_DEPS_PATH} ${FILE}"

	tar xjf ${FILE} -C ${EXTRACTED_DEPS_PATH} || {
		echo "Failed to extract ${FILE}"
		exit 1
	}
	mv ${EXTRACTED_DEPS_PATH}/juju-dqlite-static-lib-deps ${EXTRACTED_DEPS_ARCH_PATH} || {
		echo "Failed to move ${EXTRACTED_DEPS_PATH}/juju-dqlite-static-lib-deps to ${EXTRACTED_DEPS_ARCH_PATH}"
		exit 1
	}
}
