#!/bin/bash

set -e

source "$(dirname $0)/../env.sh"

check_dependencies sha256sum

sha() {
	case ${BUILD_ARCH} in
		amd64) echo "26e53cff25b2be965620c181e8e2ef6fbf74556762e7b5ce6ae71bf48e7712b7" ;;
		arm64) echo "1f0d328f785f43dd6547ebcb44d13b634aa6327bb8de85cb7052baa710be1f64" ;;
		s390x) echo "1d8c88e4c1919f648ee2fe857b139964000216c214226622d4b46ebe913bfc69" ;;
		ppc64le) echo "a40b3ba7d158b79c3a4ba8e0826c8004e9ed48a7281e850ab5d1afd0ded854ad" ;;
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
