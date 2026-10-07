#!/usr/bin/env bash
set -euf

# Path variables
BASE_DIR=$(realpath $(dirname "$0"))
PROJECT_DIR=${PROJECT_DIR:-${BASE_DIR}}
BUILD_DIR=${BUILD_DIR:-${PROJECT_DIR}/_build}
JUJUD_BIN_DIR=${JUJUD_BIN_DIR:-${BUILD_DIR}/bin}

# Versioning variables
JUJU_BUILD_NUMBER=${JUJU_BUILD_NUMBER:-}

# OCI variables
OCI_BUILDER=${OCI_BUILDER:-docker}
PUSH_OCI_REGISTRY=${PUSH_OCI_REGISTRY:-ghcr.io/juju}

# Docker variables
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
    (cd "${PROJECT_DIR}" && go run scripts/version/main.go)
}

operator_image_release_path() {
    juju_version=$(juju_version)
    echo "${PUSH_OCI_REGISTRY}/jujud-operator:$(_image_version $juju_version)"
}

operator_image_path() {
    juju_version=$(juju_version)
    if [[ -z "${JUJU_BUILD_NUMBER}" ]] || [[ ${JUJU_BUILD_NUMBER} -eq 0 ]]; then
        operator_image_release_path
    else
        echo "${PUSH_OCI_REGISTRY}/jujud-operator:$(_image_version "$juju_version").${JUJU_BUILD_NUMBER}"
    fi
}

# operator_image_lib_dir echoes the per-platform jujud runtime library staging
# directory consumed by the dynamic-image-layout contract in caas/Dockerfile.
operator_image_lib_dir() {
    echo "${BUILD_DIR}/$(echo "$1" | sed 's/\//_/g')/lib"
}

# Sonames provided by the Ubuntu base image are never staged into the
# image's /opt/lib: the image's own runtime must not be shadowed, and
# staging them would only duplicate what the base already resolves. The
# list covers the glibc family and the loader (libc6 and the loader come
# from the base image), plus liblz4 and libsqlite3: the controller snap
# does not bundle liblz4 (it comes from the snap's core26 base) and the
# published operator image's Ubuntu base carries both (verified against
# public.ecr.aws/ubuntu/ubuntu:24.04). Everything else in the jujud
# closure must be staged into _build/<os>_<arch>/lib/, or the image build
# fails.
base_libs='^(ld-linux|ld64|libc\.so|libm\.so|libpthread|libdl|librt|libresolv|libgcc_s\.so|liblz4\.so|libsqlite3\.so)'

# elf_needed_sonames echoes the NEEDED shared-library sonames of an ELF
# binary, one per line. readelf is architecture-neutral, so it is safe
# against foreign-architecture ELFs where host ldd is not.
elf_needed_sonames() {
    readelf -d "$1" | sed -n 's/.*Shared library: \[\([^]]*\)\]/\1/p'
}

# stage_operator_image_snap_payload stages the per-platform operator-image
# payload (_build/<os>_<arch>/bin/jujud and _build/<os>_<arch>/lib/) from
# the locally built controller snap, giving every operator image the same
# provenance rule as the release path: the jujud binary and its bundled
# Dqlite shared libraries come from the controller snap (built by
# snaps/jujud/snapcraft.yaml, via the smart jujud-snap-build flow), never
# from a source-built binary linked against host libraries. The snap
# payload's lib/ tree carries the primed libdqlite, libuv and libsqlite3
# soname links and real files; soname links are preserved as links so the
# loader resolves them by soname the same way it does inside the snap.
# Sonames the image base provides (see base_libs) are staged as they are
# found - the completeness of the staged closure is verified separately by
# require_operator_image_libs, which walks the NEEDED closure of the
# staged binary and every staged library.
stage_operator_image_snap_payload() {
    platform="${1:-$(go env GOOS)/$(go env GOARCH)}"
    os=$(echo "$platform" | cut -d/ -f1)
    arch=$(echo "$platform" | cut -d/ -f2)
    if [ "${os}" != "$(go env GOOS)" ] || [ "${arch}" != "$(go env GOARCH)" ]; then
        echo "operator image staging: ${platform} is a foreign platform on this host;" >&2
        echo "operator image payloads are extracted from the locally built controller snap," >&2
        echo "which is a native build. Build on a native ${arch} host," >&2
        echo "or pre-stage the payload (OPERATOR_IMAGE_BUILD_SRC=false)." >&2
        exit 1
    fi
    snap_arch=$(echo "${arch}" | sed 's/ppc64le/ppc64el/')
    snap_version=$(sed -n 's/^version: *//p' "${PROJECT_DIR}/snaps/jujud/snapcraft.yaml" | tr -d '"' | head -n1)
    snap_file="${BUILD_DIR}/snap/jujud_${snap_version}_${snap_arch}.snap"
    if [ ! -f "${snap_file}" ]; then
        echo "operator image staging: no controller snap found at ${snap_file};" >&2
        echo "run 'make jujud-snap-build' first" >&2
        exit 1
    fi
    platform_dir="${BUILD_DIR}/${os}_${arch}"
    bin_dir="${platform_dir}/bin"
    lib_dir=$(operator_image_lib_dir "$platform")
    if ! command -v unsquashfs >/dev/null 2>&1; then
        echo "operator image staging: unsquashfs (squashfs-tools) is required to extract the controller snap payload" >&2
        exit 1
    fi
    tmp_root=$(mktemp -d)
    cleanup() {
        rm -rf "${tmp_root}"
    }
    trap cleanup EXIT
    unsquashfs -no-progress -d "${tmp_root}/root" "${snap_file}" 'bin/jujud' 'lib/*' >/dev/null
    if [ ! -f "${tmp_root}/root/bin/jujud" ]; then
        echo "operator image staging: the controller snap payload does not carry bin/jujud" >&2
        exit 1
    fi
    mkdir -p "${bin_dir}"
    rm -rf "${lib_dir}"
    mkdir -p "${lib_dir}"
    cp "${tmp_root}/root/bin/jujud" "${bin_dir}/jujud"
    staged=0
    while IFS= read -r lib_src; do
        real_src=$(readlink -f "${lib_src}")
        real_name=$(basename "${real_src}")
        soname=$(basename "${lib_src}")
        cp -L "${real_src}" "${lib_dir}/${real_name}"
        if [ "${soname}" != "${real_name}" ]; then
            ln -sf "${real_name}" "${lib_dir}/${soname}"
        fi
        staged=$((staged + 1))
    done < <(find "${tmp_root}/root/lib" \( -type f -o -type l \) -name '*.so*' 2>/dev/null)
    if [ "${staged}" -eq 0 ]; then
        echo "operator image staging: the controller snap payload carries no shared libraries;" >&2
        echo "the juju-dynamic-image-layout contract requires the snap's bundled Dqlite closure" >&2
        exit 1
    fi
    echo "operator image staging: staged ${bin_dir}/jujud and ${staged} libraries from ${snap_file}"
}

# stage_operator_image_libs was removed with the source-build image
# staging: every operator image now consumes the controller snap payload
# (stage_operator_image_snap_payload locally, pre-staged CI payloads, or
# the release snap extraction), validated by require_operator_image_libs.

# require_operator_image_libs checks that the runtime libraries jujud needs
# are already present in the build payload's lib/ directory. This runs when
# OPERATOR_IMAGE_BUILD_SRC is false: instead of compiling jujud here, the CI
# payload delivers the binary together with its shared libraries under
# _build/<os>_<arch>/lib/. Nothing is rebuilt or replaced here - we only
# verify what was delivered. Every library the jujud binary needs is
# checked, including libraries needed by other staged libraries, except
# the C runtime the Ubuntu base image already provides (see base_libs).
# A missing library fails the image build here with a clear message,
# rather than producing an image whose jujud would fail to start.
require_operator_image_libs() {
    platform="${1:-}"
    os=$(echo "$platform" | cut -d/ -f1)
    arch=$(echo "$platform" | cut -d/ -f2)
    platform_dir="${BUILD_DIR}/${os}_${arch}"
    jujud_bin="${platform_dir}/bin/jujud"
    lib_dir=$(operator_image_lib_dir "$platform")
    if [ ! -d "${lib_dir}" ]; then
        echo "operator image build: missing pre-staged jujud library closure at ${lib_dir};" >&2
        echo "provide it as part of the build payload (source-build closure or snap extraction)" >&2
        exit 1
    fi
    if [ ! -f "${jujud_bin}" ]; then
        echo "operator image build: missing jujud binary at ${jujud_bin}" >&2
        exit 1
    fi
    if ! command -v readelf >/dev/null 2>&1; then
        echo "operator image build: readelf (binutils) is required to verify the pre-staged jujud closure" >&2
        exit 1
    fi
    pending=$(elf_needed_sonames "${jujud_bin}")
    seen=""
    missing=""
    while [ -n "${pending}" ]; do
        soname=$(echo "${pending}" | head -n 1)
        pending=$(echo "${pending}" | tail -n +2)
        if [ -z "${soname}" ]; then
            continue
        fi
        case " ${seen} " in
            *" ${soname} "*) continue ;;
        esac
        seen="${seen} ${soname}"
        if echo "${soname}" | grep -Eq "${base_libs}"; then
            continue
        fi
        if [ ! -e "${lib_dir}/${soname}" ]; then
            missing="${missing} ${soname}"
            continue
        fi
        pending="${pending}
$(elf_needed_sonames "${lib_dir}/${soname}")"
    done
    if [ -n "${missing}" ]; then
        echo "operator image build: the pre-staged jujud library closure at ${lib_dir} is incomplete;" >&2
        echo "missing libraries:${missing}" >&2
        echo "provide a complete closure as part of the build payload" >&2
        exit 1
    fi
}


# build_push_operator_image is responsible for doing the heavy lifting when it
# comes time to build the Juju oci operator image. This function can also build
# the operator image for multiple architectures at once. Takes 2 arguments:
# - $1 space separated list of os/arch to build the image for. Follow the GO
#   idiom for naming. Example "linux/amd64 linux/arm64". The only supported OS
#   is linux at the moment. If no argument is provided defaults to GOOS & GOARCH
# - $2 true or false value on if the resultant image(s) should be pushed to the
#   registry
#
# The jujud binary and its runtime library closure are never built or staged
# here: they are a pre-staged input (staged from the locally built controller
# snap by the image-check target locally, delivered by the QA payload unpack
# or the release snap extraction in CI), verified by require_operator_image_libs.
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

    # Verify the staged jujud payload for every image platform, per the
    # dynamic-image-layout contract in caas/Dockerfile. With
    # OPERATOR_IMAGE_BUILD_SRC=true (local) the payload was just staged by
    # stage_operator_image_snap_payload; with false (CI) it was delivered
    # pre-staged by the QA payload unpack or the release snap extraction.
    # Nothing is built or staged here - only verified.
    for platform in $build_multi_osarch; do
        require_operator_image_libs "${platform}"
    done

    push_image=${2:-"false"}


    build_multi_osarch=$(echo $build_multi_osarch | sed 's/ /,/g')

    WORKDIR=$(_make_docker_staging_dir)
    cp "${PROJECT_DIR}/caas/Dockerfile" "${WORKDIR}/"
    rm -rf "${BUILD_DIR}/controller-wrappers"
    cp -r "${PROJECT_DIR}/caas/controller-wrappers" "${BUILD_DIR}/controller-wrappers"
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
  # Copy all the lts that are available
  for (( i = 18; ; i += 2 )); do
    if "$DOCKER_BIN" pull "ghcr.io/juju/charm-base:ubuntu-$i.04" ; then
      "$DOCKER_BIN" tag "ghcr.io/juju/charm-base:ubuntu-$i.04" "${PUSH_OCI_REGISTRY}/charm-base:ubuntu-$i.04"
      "$DOCKER_BIN" push "${PUSH_OCI_REGISTRY}/charm-base:ubuntu-$i.04"
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

# apt_update runs 'apt-get update' for the provisioning targets.
apt_update() {
    sudo apt-get update
}
