#!/bin/sh
set -euf

# Path variables
BASE_DIR=$(realpath $(dirname "$0"))
PROJECT_DIR=${PROJECT_DIR:-${BASE_DIR}}
BUILD_DIR=${BUILD_DIR:-${PROJECT_DIR}/_build}
JUJUD_BIN_DIR=${JUJUD_BIN_DIR:-${BUILD_DIR}/bin}

# Versioning variables
JUJU_BUILD_NUMBER=${JUJU_BUILD_NUMBER:-}

# Docker variables
DOCKER_USERNAME=${DOCKER_USERNAME:-jujusolutions}
DOCKER_STAGING_DIR="${BUILD_DIR}/docker-staging"
DOCKER_BIN=${DOCKER_BIN:-$(which docker || true)}

# _make_docker_staging_dir is responsible for ensuring that there exists a
# Docker staging directory under the build path. The staging directory's path
# is returned as the output of this function.
_make_docker_staging_dir() {
  dir="${PROJECT_DIR}/_build/docker-staging"
  rm -rf "$dir"
  mkdir -p "$dir"
  echo "$dir"
}

_juju_version() {
    echo "$1" | grep -E -o "^[[:digit:]]{1,9}\.[[:digit:]]{1,9}(\.|-[[:alpha:]]+)[[:digit:]]{1,9}(\.[[:digit:]]{1,9})?"
}
_strip_build_version() {
    echo "$1" | grep -E -o "^[[:digit:]]{1,9}\.[[:digit:]]{1,9}(\.|-[[:alpha:]]+)[[:digit:]]{1,9}"
}
_image_version() {
    _strip_build_version "$(_juju_version $@)"
}

microk8s_operator_update() {
  echo "Uploading image $(operator_image_path) to microk8s"
  # For macos we have to push the image into the microk8s multipass vm because
  # we can't use the ctr to stream off the local machine.
  if [ $(uname) = "Darwin" ]; then
    tmp_docker_image="/tmp/juju-operator-image-${RANDOM}.image"
    docker save $(operator_image_path) | multipass transfer - microk8s-vm:${tmp_docker_image}
    microk8s ctr --namespace k8s.io image import ${tmp_docker_image}
    multipass exec microk8s-vm rm "${tmp_docker_image}"
    return
  fi

  # Linux we can stream the file like normal.
  docker save "$(operator_image_path)" | microk8s.ctr --namespace k8s.io image import -
}

juju_version() {
    echo $(go run ${PROJECT_DIR}/version/helper/main.go)
}

operator_image_release_path() {
    juju_version=$(juju_version)
    echo "${DOCKER_USERNAME}/jujud-operator:$(_image_version $juju_version)"
}

operator_image_path() {
    juju_version=$(juju_version)
    if [ -z "${JUJU_BUILD_NUMBER}" ] || [ ${JUJU_BUILD_NUMBER} -eq 0 ]; then
        operator_image_release_path "$juju_version"
    else
        echo "${DOCKER_USERNAME}/jujud-operator:$(_image_version "$juju_version").${JUJU_BUILD_NUMBER}"
    fi
}


# build_operator_image is responsible for doing the heavy lifiting when it
# comes time to build the Juju oci operator image. This function can also build
# the operator image for multiple architectures at once. Takes 2 arguments
# - $1 juju-version to take the image
# - $2 comma seperated list of os/arch to build the image for. Follow the GO
#   idiom for naming. Example linux/amd64,linux/arm64. The only supported OS
#   is linux at the moment. If no argument is provided defaults to GOOS & GOARCH
build_operator_image() {
    build_multi_osarch=${1-""}
    if [ -z "$build_multi_osarch" ]; then
      build_multi_osarch="$(go env GOOS)/$(go env GOARCH)"
    fi

    WORKDIR=$(_make_docker_staging_dir)
    cp "${PROJECT_DIR}/caas/Dockerfile" "${WORKDIR}/"
    cp "${PROJECT_DIR}/caas/requirements.txt" "${WORKDIR}/"
    for build_osarch in ${build_multi_osarch}; do
      tar cf - -C "${BUILD_DIR}" . | DOCKER_BUILDKIT=1 "${DOCKER_BIN}" build \
          -f "docker-staging/Dockerfile" \
          --platform "$build_osarch" \
          -t "$(operator_image_path)" -
    done
    if [ "$(operator_image_path)" != "$(operator_image_release_path)" ]; then
        "${DOCKER_BIN}" tag "$(operator_image_path)" "$(operator_image_release_path $juju_version)"
    fi
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
