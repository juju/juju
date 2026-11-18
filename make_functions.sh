#!/usr/bin/env bash
set -euf

# Path variables
BASE_DIR=$(realpath "$(dirname "$0")")
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
    _strip_build_version "$(_juju_version "$@")"
}

microk8s_operator_update() {
    echo "Uploading image $(operator_image_path) to microk8s"
    # For macos we have to push the image into the microk8s multipass vm because
    # we can't use the ctr to stream off the local machine.
    if [[ $(uname) = "Darwin" ]]; then
        tmp_docker_image="/tmp/juju-operator-image-${RANDOM}.image"
        "${DOCKER_BIN}" save "$(operator_image_path)" | multipass transfer - microk8s-vm:${tmp_docker_image}
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

# The require_operator_image_libs verification step skips sonames that
# the Ubuntu base image already provides (glibc, the loader, liblz4, and
# libsqlite3): these are excluded from the NEEDED closure walk so an
# absent base library does not fail the build. The controller snap may
# still bundle and stage some of these (e.g. libsqlite3 for Dqlite
# version compatibility), which the staging loop copies unconditionally;
# when the snap's library shadows the base image the operator's RPATH
# favours the staged copy. The list covers the glibc family and the
# loader (libc6 and the loader come from the base image), plus liblz4
# (the controller snap does not bundle it; it comes from the snap's
# core26 base) and libsqlite3 (the published operator image's Ubuntu 24.04
# base carries it, verified against public.ecr.aws/ubuntu/ubuntu:24.04).
# Everything else in the jujud closure must be resolved in the staged
# _build/<os>_<arch>/lib/, or the image build fails.
base_libs='^(ld-linux|ld64|libc\.so|libm\.so|libpthread|libdl|librt|libresolv|libgcc_s\.so|liblz4\.so|libsqlite3\.so)'

# elf_needed_sonames echoes the NEEDED shared-library sonames of an ELF
# binary, one per line. readelf is architecture-neutral, so it is safe
# against foreign-architecture ELFs where host ldd is not.
elf_needed_sonames() {
    readelf -d "$1" | sed -n 's/.*Shared library: \[\([^]]*\)\]/\1/p'
}

# require_operator_image_libs checks that the runtime libraries jujud needs
# are already present in the build payload's lib/ directory. The Docker
# image path consumes a payload produced outside this Makefile (the QA
# payload unpack or the release snap extraction): instead of compiling
# jujud here, the payload delivers the binary together with its shared
# libraries under _build/<os>_<arch>/lib/. Nothing is rebuilt or replaced
# here - we only verify what was delivered. Every library the jujud binary
# needs is checked, including libraries needed by other staged libraries,
# except the C runtime the Ubuntu base image already provides (see
# base_libs). A missing library fails the image build here with a clear
# message, rather than producing an image whose jujud would fail to start.
require_operator_image_libs() {
    platform="${1:-}"
    os=$(echo "$platform" | cut -d/ -f1)
    arch=$(echo "$platform" | cut -d/ -f2)
    platform_dir="${BUILD_DIR}/${os}_${arch}"
    jujud_bin="${platform_dir}/bin/jujud"
    lib_dir=$(operator_image_lib_dir "$platform")
    if [ ! -d "${lib_dir}" ]; then
        echo "operator image build: missing pre-staged jujud library closure at ${lib_dir};" >&2
        echo "provide it as part of the build payload (QA payload unpack or release snap extraction)" >&2
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
# here: they are a pre-staged input delivered by the QA payload unpack or the
# release snap extraction, verified by require_operator_image_libs.
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
    # dynamic-image-layout contract in caas/Dockerfile. The payload is
    # delivered pre-staged by the QA payload unpack or the release snap
    # extraction; the Docker image path never builds or stages it here.
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

# ---------------------------------------------------------------------------
# Chiselled operator rock
# ---------------------------------------------------------------------------

# operator_rock_arch echoes the native Debian architecture name. Rockcraft
# uses Debian architecture names, so ppc64le is reported as ppc64el.
operator_rock_arch() {
    echo "${OPERATOR_ROCK_ARCH:-$(dpkg --print-architecture)}"
}

operator_rock_dir() {
    echo "${OPERATOR_ROCK_DIR:-${PROJECT_DIR}/rocks/jujud-operator}"
}

operator_rock_build_dir() {
    echo "${OPERATOR_ROCK_BUILD_DIR:-${BUILD_DIR}/rocks}"
}

# operator_rock_version is the rock archive version. It matches the image
# tag portion of operator_image_path (no build number).
operator_rock_version() {
    _strip_build_version "$(juju_version)"
}

operator_rock_archive() {
    echo "$(operator_rock_build_dir)/jujud-operator_$(operator_rock_version)_$(operator_rock_arch).rock"
}

# require_operator_rock_tools checks the tools that pack a rock: the
# Rockcraft CLI and a reachable LXD, since Rockcraft builds in a managed
# LXD instance. It never installs snaps, changes the LXD configuration,
# or falls back to Docker. Importing a packed rock needs neither the CLI
# nor LXD; see require_operator_rock_import_tools.
require_operator_rock_tools() {
    missing=""
    for tool in "${ROCKCRAFT:-rockcraft}" lxc; do
        if ! command -v "$tool" >/dev/null 2>&1; then
            missing="${missing} ${tool}"
        fi
    done
    if [ -n "${missing}" ]; then
        echo "operator rock: missing required tools:${missing}" >&2
        echo "install Rockcraft and LXD first;" >&2
        echo "the rock targets do not install tools or fall back to the Docker image" >&2
        exit 1
    fi
    if ! timeout 10 lxc info >/dev/null 2>&1; then
        echo "operator rock: LXD is not reachable" >&2
        echo "Rockcraft builds in a managed LXD instance; start or initialise LXD first" >&2
        exit 1
    fi
}

# require_operator_rock_import_tools checks the tools that import a packed
# rock into the local container store: the skopeo binary (Rockcraft also
# provides it as rockcraft.skopeo) and the local container runtime. Neither
# the Rockcraft CLI nor LXD is needed for the import.
require_operator_rock_import_tools() {
    skopeo="${ROCKCRAFT_SKOPEO:-rockcraft.skopeo}"
    if ! command -v "$skopeo" >/dev/null 2>&1; then
        echo "operator rock: missing required tool: ${skopeo}" >&2
        echo "install Rockcraft (which provides rockcraft.skopeo) first;" >&2
        echo "the rock targets do not install tools or fall back to the Docker image" >&2
        exit 1
    fi
    require_operator_image_runtime
}

# require_operator_image_runtime checks the local container runtime used to
# import and tag the packed rock.
require_operator_image_runtime() {
    case "${OCI_BUILDER}" in
        docker | podman)
            ;;
        *)
            echo "operator image: unsupported OCI_BUILDER=${OCI_BUILDER}; expected docker or podman" >&2
            exit 1
            ;;
    esac
    if [ -z "${DOCKER_BIN}" ] || ! command -v "${DOCKER_BIN}" >/dev/null 2>&1; then
        echo "operator image: ${OCI_BUILDER} is not available" >&2
        exit 1
    fi
}

# require_native_operator_rock_platform rejects foreign and multi-platform
# requests: a local rock is a native, single-architecture build.
require_native_operator_rock_platform() {
    requested="${1:-}"
    if [ -z "${requested}" ]; then
        requested="linux/$(go env GOARCH)"
    fi
    case "${requested}" in
        *" "* | *,*)
            echo "operator rock: local rock builds are single-platform only" >&2
            echo "requested platforms: ${requested}" >&2
            exit 1
            ;;
    esac
    requested_arch=$(echo "${requested}" | sed -e 's|^linux/||' -e 's/ppc64le/ppc64el/')
    native_arch=$(operator_rock_arch)
    if [ "${requested_arch}" != "${native_arch}" ]; then
        echo "operator rock: local rock builds are native only" >&2
        echo "requested ${requested}, host architecture is ${native_arch}" >&2
        echo "build on a native ${requested_arch} host, or use the Docker image path" >&2
        exit 1
    fi
}

# pack_operator_rock packs the native rock and moves the archive to the
# build output directory. It never builds a controller snap or stages the
# payload of the Docker image.
pack_operator_rock() {
    require_operator_rock_tools
    require_native_operator_rock_platform "${1:-}"

    arch=$(operator_rock_arch)
    name="jujud-operator_$(operator_rock_version)_${arch}.rock"
    archive="$(operator_rock_dir)/${name}"
    output_archive="$(operator_rock_build_dir)/${name}"

    mkdir -p "$(operator_rock_build_dir)"
    rm -f "${archive}"

    echo "Packing ${name} with Rockcraft for native architecture ${arch}"
    (
        cd "$(operator_rock_dir)"
        ROCKCRAFT_EXPERIMENTAL_MONOREPO=1 "${ROCKCRAFT:-rockcraft}" pack --build-for "${arch}"
    )

    if [ ! -f "${archive}" ]; then
        echo "operator rock: Rockcraft did not produce ${archive}" >&2
        exit 1
    fi
    mv -f "${archive}" "${output_archive}"
    echo "Packed ${output_archive}"
}

# import_operator_rock_image imports the packed OCI archive into the local
# Docker or Podman store under the unchanged operator_image_path tag.
import_operator_rock_image() {
    require_operator_rock_import_tools

    archive=$(operator_rock_archive)
    tag=$(operator_image_path)
    if [ ! -f "${archive}" ]; then
        echo "operator image: no rock archive at ${archive}" >&2
        echo "run 'make operator-rock' first" >&2
        exit 1
    fi

    case "${OCI_BUILDER}" in
        docker)
            transport="docker-daemon:${tag}"
            ;;
        podman)
            transport="containers-storage:${tag}"
            ;;
    esac

    echo "Importing ${archive} as ${tag}"
    "${ROCKCRAFT_SKOPEO:-rockcraft.skopeo}" --insecure-policy \
        copy "oci-archive:${archive}" "${transport}"
    echo "Tagged ${tag} in the local ${OCI_BUILDER} store"
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

# snap_dqlite_pin reads the controller snap's Dqlite pin from the snapcraft
# dqlite part. The part builds upstream Dqlite from a pinned source commit
# annotated with its upstream version ('source-commit: <sha> # v1.18.7').
snap_dqlite_pin() {
    sed -n 's/^[[:space:]]*source-commit:.*#[[:space:]]*\(v[0-9][0-9.]*\)[[:space:]]*$/\1/p' \
        "${PROJECT_DIR}/snaps/jujud/snapcraft.yaml" | head -n1
}

# install_dqlite_dev_packages installs the Dqlite development packages the
# dynamic jujud build links against, pinned to the controller snap's Dqlite
# version. The snapcraft dqlite part builds upstream Dqlite from a pinned
# source commit; the local build must link against the same upstream version
# so the fast-patch library-pairing rule (the host link-time Dqlite matches
# the snap-bundled Dqlite) holds by construction instead of by documentation.
# The series-suffixed real dev package (libdqlite<series>-dev, e.g.
# libdqlite1.18-dev) is installed rather than the libdqlite-dev meta: the
# meta's resolved version depends on which archive wins the resolution
# (the distro archive also carries a libdqlite-dev), while the series
# package is the actual header and library carrier. The installed upstream
# version is verified against the snap pin; a mismatch fails loudly with
# the remedy instead of silently linking a drifted Dqlite.
install_dqlite_dev_packages() {
    pin=$(snap_dqlite_pin)
    if [ -z "${pin}" ]; then
        echo "install-dqlite-dependencies: cannot read the snap's Dqlite pin from" >&2
        echo "${PROJECT_DIR}/snaps/jujud/snapcraft.yaml;" >&2
        echo "expected a 'source-commit: <sha> # v<version>' line in the dqlite part" >&2
        exit 1
    fi
    version="${pin#v}"
    series=$(printf '%s' "${version}" | cut -d. -f1,2)
    dev_pkg="libdqlite${series}-dev"
    echo "Installing ${dev_pkg} from ppa:dqlite/dev (snap pin: ${pin})"
    sudo apt-get --yes install gcc "${dev_pkg}" libuv1-dev liblz4-dev
    installed=$(dpkg-query -W -f='${Version}' "${dev_pkg}" 2>/dev/null || true)
    if [ -z "${installed}" ]; then
        echo "install-dqlite-dependencies: ${dev_pkg} is not installed after apt-get install" >&2
        exit 1
    fi
    upstream=$(printf '%s' "${installed}" | sed -E 's/[-+~].*$//')
    if [ "${upstream}" != "${version}" ]; then
        echo "install-dqlite-dependencies: ${dev_pkg} resolved to ${installed} but the snap pins ${pin}." >&2
        echo "Raise the snap's dqlite source-commit pin to the PPA version, or install the exact" >&2
        echo "pinned version (${dev_pkg}=${version}~*); a local build linked against a different" >&2
        echo "Dqlite violates the fast-patch library-pairing rule." >&2
        exit 1
    fi
    echo "Native Dqlite: ${dev_pkg} ${installed} (matches the snap pin ${pin}, ppa:dqlite/dev)"
}
