#!/bin/bash

set -e

source "$(dirname $0)/../env.sh"

check_dependencies sha256sum

sha() {
	case ${BUILD_ARCH} in
		amd64) echo "a83fae718d97824d2ba6b611de3d9514544661a35d0a3d3384929d2118dc18ae" ;;
		arm64) echo "eb90c1a3dae19b4f159cff69c37856fc3219be5a2f7a560d8f85767bb9feac97" ;;
		s390x) echo "5b98cf389edc7060a56c452f103361c4393816e2ff6b6ff1ec73afce2bcba8d4" ;;
		ppc64le) echo "2da45f69eed2ac22255efda068e5f7679558f6a15db231d4372fa5e472ac3e91" ;;
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
