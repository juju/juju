#!/bin/sh
set -euf

# Path variables
BASE_DIR=$(realpath $(dirname "$0"))
PROJECT_DIR=${PROJECT_DIR:-${BASE_DIR}}
BUILD_DIR=${BUILD_DIR:-${PROJECT_DIR}/_build/$(go env GOOS)_$(go env GOARCH)}
JUJUD_BIN_DIR=${JUJUD_BIN_DIR:-${BUILD_DIR}/bin}

# Versioning variables
JUJU_BUILD_NUMBER=${JUJU_BUILD_NUMBER:-}

# Docker variables
DOCKER_USERNAME=${DOCKER_USERNAME:-jujusolutions}
DOCKER_STAGING_DIR="${BUILD_DIR}/docker-staging"
DOCKER_BIN=${DOCKER_BIN:-$(which docker)}

_base_image() {
    IMG_linux_amd64="amd64/ubuntu:20.04" \
    IMG_linux_arm64="arm64v8/ubuntu:20.04" \
    IMG_linux_ppc64le="ppc64le/ubuntu:20.04" \
    IMG_linux_s390x="s390x/ubuntu:20.04" \
    printenv "IMG_$(go env GOOS)_$(go env GOARCH)"
}

_juju_version() {
    "${JUJUD_BIN_DIR}/jujud" version | grep -E -o "^[[:digit:]]{1,9}\.[[:digit:]]{1,9}(\.|-[[:alpha:]]+)[[:digit:]]{1,9}(\.[[:digit:]]{1,9})?"
}
_strip_build_version() {
    echo "$1" | grep -E -o "^[[:digit:]]{1,9}\.[[:digit:]]{1,9}(\.|-[[:alpha:]]+)[[:digit:]]{1,9}"
}
_operator_image_version() {
    _strip_build_version "$(_juju_version)"
}

operator_image_release_path() {
    echo "${DOCKER_USERNAME}/jujud-operator:$(_operator_image_version)"
}
operator_image_path() {
    if [ -z "${JUJU_BUILD_NUMBER}" ] || [ ${JUJU_BUILD_NUMBER} -eq 0 ]; then
        operator_image_release_path
    else
        echo "${DOCKER_USERNAME}/jujud-operator:$(_operator_image_version).${JUJU_BUILD_NUMBER}"
    fi
}

build_operator_image() {
    rm -rf "${DOCKER_STAGING_DIR}"
    mkdir -p "${DOCKER_STAGING_DIR}"

    # Populate docker build context
    cp "${JUJUD_BIN_DIR}/jujuc" "${DOCKER_STAGING_DIR}/" || true
    cp "${JUJUD_BIN_DIR}/jujud" "${DOCKER_STAGING_DIR}/"
    cp "${PROJECT_DIR}/caas/jujud-operator-dockerfile" "${DOCKER_STAGING_DIR}/Dockerfile"
    cp "${PROJECT_DIR}/caas/jujud-operator-requirements.txt" "${DOCKER_STAGING_DIR}/"

    # Build image. We tar up the build context to support docker snap confinement.
    tar cf - -C "${DOCKER_STAGING_DIR}" . | "${DOCKER_BIN}" build --build-arg BASE_IMAGE=$(_base_image) -t "$(operator_image_path)" - 
    if [ "$(operator_image_path)" != "$(operator_image_release_path)" ]; then
        "${DOCKER_BIN}" tag "$(operator_image_path)" "$(operator_image_release_path)"
    fi

    # Cleanup
    rm -rf "${DOCKER_STAGING_DIR}"
}
