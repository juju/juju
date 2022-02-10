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
DOCKER_BIN=${DOCKER_BIN:-$(which docker || true)}

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
_image_version() {
    _strip_build_version "$(_juju_version)"
}

microk8s_operator_update() {
  echo "Uploading image $(operator_image_path) to microk8s"
  # For macos we have to push the image into the microk8s multipass vm because
  # we can't use the ctr to stream off the local machine.
  if [ $(uname) == "Darwin" ]; then
    tmp_docker_image="/tmp/juju-operator-image-${RANDOM}.image"
    docker save $(operator_image_path) | multipass transfer - microk8s-vm:${tmp_docker_image}
    microk8s ctr --namespace k8s.io image import ${tmp_docker_image}
    multipass exec microk8s-vm rm "${tmp_docker_image}"
    return
  fi

  # Linux we can stream the file like normal.
  docker save "$(operator_image_path)" | microk8s.ctr --namespace k8s.io image import -
}

operator_image_release_path() {
    echo "${DOCKER_USERNAME}/jujud-operator:$(_image_version)"
}

operator_image_path() {
    if [ -z "${JUJU_BUILD_NUMBER}" ] || [ ${JUJU_BUILD_NUMBER} -eq 0 ]; then
        operator_image_release_path
    else
        echo "${DOCKER_USERNAME}/jujud-operator:$(_image_version).${JUJU_BUILD_NUMBER}"
    fi
}

build_operator_image() {
    WORKDIR="${DOCKER_STAGING_DIR}/jujud-operator"
    rm -rf "${WORKDIR}"
    mkdir -p "${WORKDIR}"

    # Populate docker build context
    cp "${JUJUD_BIN_DIR}/jujud" "${WORKDIR}/"
    cp "${JUJUD_BIN_DIR}/jujuc" "${WORKDIR}/"
    cp "${JUJUD_BIN_DIR}/containeragent" "${WORKDIR}/"
    cp "${JUJUD_BIN_DIR}/pebble" "${WORKDIR}/"
    cp "${PROJECT_DIR}/caas/Dockerfile" "${WORKDIR}/"
    cp "${PROJECT_DIR}/caas/requirements.txt" "${WORKDIR}/"

    # Build image. We tar up the build context to support docker snap confinement.
    tar cf - -C "${WORKDIR}" . | "${DOCKER_BIN}" build \
        --build-arg BASE_IMAGE=$(_base_image) \
        -t "$(operator_image_path)" - 
    if [ "$(operator_image_path)" != "$(operator_image_release_path)" ]; then
        "${DOCKER_BIN}" tag "$(operator_image_path)" "$(operator_image_release_path)"
    fi

    # Cleanup
    rm -rf "${WORKDIR}"
}

wait_for_dpkg() {
    # Just in case, wait for cloud-init.
    cloud-init status --wait 2> /dev/null || true
    while sudo lsof /var/lib/dpkg/lock-frontend 2> /dev/null; do
        echo "Waiting for dpkg lock..."
        sleep 10
    done
    while sudo lsof /var/lib/apt/lists/lock 2> /dev/null; do
        echo "Waiting for apt lock..."
        sleep 10
    done
}
