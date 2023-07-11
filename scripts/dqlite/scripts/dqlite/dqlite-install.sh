#!/bin/bash

set -e

source "$(dirname $0)/../env.sh"

check_dependencies sha256sum

sha() {
	case ${BUILD_ARCH} in
		amd64) echo "baa172b25826ed1f494b924c9d8fa9b4a53b96392f903d222babf79de1c4b72e" ;;
		arm64) echo "afe8c7b1d805c2e63d498606e43c503cc42e09716e70c4819702ccc0422b65d9" ;;
		s390x) echo "355b9b42a3b1a5e4859785b002804856bcee1406f0eab3684a1121fdd459ef1f" ;;
		ppc64le) echo "7febbd07617f20d490384abb75f7309cd9b6fe0db445de755b7068f8a4a2cfeb" ;;
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
