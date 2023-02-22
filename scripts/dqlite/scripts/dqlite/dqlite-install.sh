#!/bin/bash

set -e

source "$(dirname $0)/../env.sh"

sha() {
	case ${BUILD_ARCH} in
		amd64) echo "c6d93a819647db2d2d69a7e942d864f33d95d2c6476f80e31234d56354a05185" ;;
		arm64) echo "7a491488dd8a0f4ce3c5f4b44c3a10b5c11eaffdda937033fc9a9ed8d9d0ec49" ;;
		s390x) echo "448be80e5281aa4f011c1bd3c50253dbf70e3e72da20440c477bc17b16560152" ;;
		ppc64le) echo "e648377f0eb07eb9edac66e48d55ca7a0ea9e7e437ca5c1cd6bf1cb7d6bbc143" ;;
		*) { echo "Unsupported arch ${BUILD_ARCH}."; exit 1; } ;;
	esac
}

FILE="$(mktemp -d)/latest-dqlite-deps-${BUILD_ARCH}.tar.bz2"

retrieve() {
	local filenames sha

	sha=${1}

	filenames=( "${sha}.tar.bz2" "latest-dqlite-deps-${BUILD_ARCH}.tar.bz2" )
	for name in "${filenames[@]}"; do
		echo "Retrieving ${name}"
		curl --fail -o ${FILE} -s https://dqlite-static-libs.s3.amazonaws.com/${name} && return || {
			echo " + Failed to retrieve ${name}";
			rm -f ${FILE} || true;
			true;
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

    tar xjf ${FILE} -C ${EXTRACTED_DEPS_PATH} || { echo "Failed to extract ${FILE}"; exit 1; }
    mv ${EXTRACTED_DEPS_PATH}/juju-dqlite-static-lib-deps ${EXTRACTED_DEPS_ARCH_PATH} || { echo "Failed to move ${EXTRACTED_DEPS_PATH}/juju-dqlite-static-lib-deps to ${EXTRACTED_DEPS_ARCH_PATH}"; exit 1; }
}
