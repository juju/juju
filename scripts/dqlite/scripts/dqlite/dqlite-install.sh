#!/bin/bash

set -e

source "$(dirname $0)/../env.sh"

sha() {
	case ${BUILD_ARCH} in
		amd64) echo "124a578c8bd63d9288093f4de4aaffa09c034d6641065cd079e446ac91b1b611" ;;
		arm64) echo "0943427d17dce0dc9d7da7777dd1f6ed8d5cea4d6c9d872411075181b9299d7e" ;;
		s390x) echo "690240895b765eb78a0a00ebc148361b947b64483e636a76d9d033780d5ef758" ;;
		ppc64le) echo "b83faf851327645e9db4a2e0df28959c1b08cc1f3ef2cf5dbf67804abd061f5c" ;;
		*) { echo "Unsupported arch ${BUILD_ARCH}."; exit 1; } ;;
	esac
}

FILE="$(mktemp -d)/latest-dqlite-deps-${BUILD_ARCH}.tar.bz2"

retrieve() {
	local filenames sha

	sha=${1}

	filenames=( "${sha}.tar.bz2" )
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
