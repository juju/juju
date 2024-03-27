#!/usr/bin/env bash
set -euf

# Path variables
BASE_DIR=$(realpath $(dirname "$0"))
PROJECT_DIR=${PROJECT_DIR:-${BASE_DIR}}
BUILD_DIR=${BUILD_DIR:-${PROJECT_DIR}/_build}
JUJUD_BIN_DIR=${JUJUD_BIN_DIR:-${BUILD_DIR}/bin}

# Versioning variables
JUJU_BUILD_NUMBER=${JUJU_BUILD_NUMBER:-$(cat ${BUILD_DIR}/build.number)}
JUJU_DB_VERSION=${JUJU_DB_VERSION:-}

# Docker variables
OCI_BUILDER=${OCI_BUILDER:-docker}
DOCKER_USERNAME=${DOCKER_USERNAME:-docker.io/jujusolutions}
DOCKER_BUILDX_CONTEXT=${DOCKER_BUILDX_CONTEXT:-juju-make}
DOCKER_STAGING_DIR="${BUILD_DIR}/docker-staging"
DOCKER_BIN=${DOCKER_BIN:-$(which ${OCI_BUILDER} || true)}

readonly docker_staging_dir="docker-staging"

# _make_docker_staging_dir is responsible for ensuring that there exists a
# Docker staging directory under the build path. The staging directory's path
# is returned as the output of this function.
_make_docker_staging_dir() {
    dir="${BUILD_DIR}/${docker_staging_dir}"
    rm -rf "$dir"
    mkdir -p "$dir"
    echo "$dir"
}

microk8s_operator_update() {
    echo "Uploading image $(operator_image_path) to microk8s"
    # For macos we have to push the image into the microk8s multipass vm because
    # we can't use the ctr to stream off the local machine.
    if [[ $(uname) = "Darwin" ]]; then
        tmp_docker_image="/tmp/juju-operator-image-${RANDOM}.image"
        "${DOCKER_BIN}" save $(operator_image_path) | multipass transfer - microk8s-vm:${tmp_docker_image}
        microk8s ctr --namespace k8s.io image import ${tmp_docker_image}
        multipass exec microk8s-vm rm "${tmp_docker_image}"
        return
    fi

    # Linux we can stream the file like normal.
    "${DOCKER_BIN}" save "$(operator_image_path)" | microk8s.ctr --namespace k8s.io image import -
}

juju_version() {
    (cd "${PROJECT_DIR}" && GOOS=$(go env GOHOSTOS) GOARCH=$(go env GOHOSTARCH) CGO_ENABLED=0 go run -ldflags "-X github.com/juju/juju/version.build=${JUJU_BUILD_NUMBER}" version/helper/main.go)
}

operator_image_path() {
    juju_version=$(juju_version)
    echo "${DOCKER_USERNAME}/jujud-operator:$(juju_version)"
}


# build_push_operator_image is responsible for doing the heavy lifting when it
# comes time to build the Juju oci operator image. This function can also build
# the operator image for multiple architectures at once. Takes 2 arguments that
# describe one or more platforms to build for and whether to push the image.
# - $1 space seperated list of os/arch to build the image for. Follow the GO
#   idiom for naming. Example "linux/amd64 linux/arm64". The only supported OS
#   is linux at the moment. If no argument is provided defaults to GOOS & GOARCH
# - $2 true or false value on if the resultant image(s) should be pushed to the
#   registry
build_push_operator_image() {
    build_multi_osarch=${1-""}
    if [[ -z "$build_multi_osarch" ]]; then
        build_multi_osarch="$(go env GOOS)/$(go env GOARCH)"
    fi

    # We need to find any ppc64el references and move the build artefacts over
    # to ppc64le so that it works with Docker.
    for platform in $build_multi_osarch; do
        if [[ "$platform" = *"ppc64el"* ]]; then
            echo "detected operator image build for ppc64el \"${platform}\""
            new_platform=$(echo "$platform" | sed 's/ppc64el/ppc64le/g')
            echo "changing platform \"${platform}\" to platform \"${new_platform}\""

            platform_dir="${BUILD_DIR}/$(echo "$platform" | sed 's/\//_/g')"
            new_platform_dir="${BUILD_DIR}/$(echo "$new_platform" | sed 's/\//_/g')"
            if ! [[ -d "$platform_dir" ]]; then
                echo "platform build directory \"${platform_dir}\" does not exist"
                exit 1
            fi

            echo "copying platform build directory \"${platform_dir}\" to \"${new_platform_dir}\""
            cp -r "$platform_dir" "$new_platform_dir"
        fi
    done
    build_multi_osarch=$(echo "$build_multi_osarch" | sed 's/ppc64el/ppc64le/g')

    push_image=${2:-"false"}


    build_multi_osarch=$(echo $build_multi_osarch | sed 's/ /,/g')

    WORKDIR=$(_make_docker_staging_dir)
    cp "${PROJECT_DIR}/caas/Dockerfile" "${WORKDIR}/"
    cp "${PROJECT_DIR}/caas/requirements.txt" "${WORKDIR}/"
    if [[ "${OCI_BUILDER}" = "docker" ]]; then
        output="-o type=oci,dest=${BUILD_DIR}/oci.tar.gz"
        if [[ "$push_image" = true ]]; then
            output="-o type=image,push=true"
        elif [[ $(echo "$build_multi_osarch" | wc -w) -eq 1 ]]; then
            output="-o type=docker"
        fi
        BUILDX_NO_DEFAULT_ATTESTATIONS=true DOCKER_BUILDKIT=1 "$DOCKER_BIN" buildx build \
            --builder "$DOCKER_BUILDX_CONTEXT" \
            -f "${WORKDIR}/Dockerfile" \
            -t "$(operator_image_path)" \
            --platform="$build_multi_osarch" \
            --provenance=false \
            ${output} \
            "${BUILD_DIR}"
    elif [[ "${OCI_BUILDER}" = "podman" ]]; then
        "$DOCKER_BIN" manifest rm "$(operator_image_path)" || true
        "$DOCKER_BIN" manifest create "$(operator_image_path)"
        "$DOCKER_BIN" build \
            --jobs "4" \
            -f "${WORKDIR}/Dockerfile" \
            --manifest "$(operator_image_path)" \
            --platform="$build_multi_osarch" \
            "${BUILD_DIR}"
        if [[ "$push_image" = true ]]; then
            "$DOCKER_BIN" manifest push -f v2s2 "$(operator_image_path)" "docker://$(operator_image_path)"
        fi
    else
        echo "unknown OCI_BUILDER=${OCI_BUILDER} expected docker or podman"
        exit 1
    fi
}

seed_repository() {
  set -x
  "$DOCKER_BIN" pull "docker.io/jujusolutions/juju-db:${JUJU_DB_VERSION}"
	"$DOCKER_BIN" tag "docker.io/jujusolutions/juju-db:${JUJU_DB_VERSION}" "${DOCKER_USERNAME}/juju-db:${JUJU_DB_VERSION}"
	"$DOCKER_BIN" push "${DOCKER_USERNAME}/juju-db:${JUJU_DB_VERSION}"

  # copy all the lts that are available
  for (( i = 18; ; i += 2 )); do
    if "$DOCKER_BIN" pull "docker.io/jujusolutions/charm-base:ubuntu-$i.04" ; then
      "$DOCKER_BIN" tag "docker.io/jujusolutions/charm-base:ubuntu-$i.04" "${DOCKER_USERNAME}/charm-base:ubuntu-$i.04"
      "$DOCKER_BIN" push "${DOCKER_USERNAME}/charm-base:ubuntu-$i.04"
    else
      break
    fi
  done	
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

patch_elf_with_version() {
    JUJU_VERSION_OBJ=${1-""}
    BINARY=${2-""}

    if [[ "$(go env GOOS)" == "$(go env GOHOSTOS)" && "$(go env GOARCH)" == "$(go env GOHOSTARCH)" ]]; then
        objcopy --add-section=.note.juju.version=${JUJU_VERSION_OBJ} --set-section-alignment=.note.juju.version=4 --set-section-flags=.note.juju.version=contents,noload,readonly ${BINARY} ${BINARY}
        exit $?
    fi

    case $(go env GOARCH) in

    amd64)
        gcc_arch=x86_64
        ;;

    arm64)
        gcc_arch=aarch64
        ;;

    ppc64le)
        gcc_arch=powerpc64le
        ;;

    s390x)
        gcc_arch=s390x
        ;;

    *)
        echo "Bad arch $(go env GOARCH)"
        ;;
    esac

    "$DOCKER_BIN" run -it --rm --userns=keep-id -u $(id -u):$(id -g) \
        --volume ${PROJECT_DIR}:${PROJECT_DIR} -w ${PROJECT_DIR} \
        --volume ${BUILD_DIR}:${BUILD_DIR} \
        --volume ${JUJU_VERSION_OBJ}:${JUJU_VERSION_OBJ} \
        --volume ${BINARY}:${BINARY} \
        ghcr.io/juju/cross-dev:ubuntu-22.04-go-1.21-dqlite \
        ${gcc_arch}-linux-gnu-objcopy --add-section=.note.juju.version=${JUJU_VERSION_OBJ} --set-section-alignment=.note.juju.version=4 --set-section-flags=.note.juju.version=contents,noload,readonly ${BINARY} ${BINARY}
}
