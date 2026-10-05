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

# multiarch_lib_dirs echoes the Debian multiarch library directories for a
# Go architecture. The arch-qualified Dqlite/SQLite packages installed by
# 'make install-dqlite-dependencies DQLITE_CROSS_ARCHES=<deb arch>' place
# their target-architecture shared libraries there.
multiarch_lib_dirs() {
    arch="${1:-}"
    case "${arch}" in
        amd64) triplet="x86_64-linux-gnu" ;;
        arm64) triplet="aarch64-linux-gnu" ;;
        s390x) triplet="s390x-linux-gnu" ;;
        ppc64le|ppc64el) triplet="powerpc64le-linux-gnu" ;;
        riscv64) triplet="riscv64-linux-gnu" ;;
        *)
            echo "operator image staging: no Debian multiarch library directories known for ${arch}" >&2
            return 1
            ;;
    esac
    echo "/usr/lib/${triplet} /lib/${triplet}"
}

# stage_foreign_operator_image_libs stages the runtime shared-library
# closure of a cross-built jujud whose target architecture is foreign to
# the build host. Host ldd cannot resolve a foreign ELF, so the closure is
# derived from readelf NEEDED entries (readelf is architecture-neutral)
# and every member is resolved from the Debian multiarch library
# directories populated by 'make install-dqlite-dependencies
# DQLITE_CROSS_ARCHES=<deb arch>'. Any non-base closure member that cannot
# be resolved fails the image build here.
stage_foreign_operator_image_libs() {
    platform="${1:-}"
    arch=$(echo "$platform" | cut -d/ -f2)
    jujud_bin="${BUILD_DIR}/$(echo "$platform" | sed 's/\//_/g')/bin/jujud"
    lib_dir=$(operator_image_lib_dir "$platform")

    if ! command -v readelf >/dev/null 2>&1; then
        echo "operator image staging: readelf (binutils) is required to stage a foreign-architecture jujud closure" >&2
        exit 1
    fi
    if ! lib_dirs=$(multiarch_lib_dirs "${arch}"); then
        exit 1
    fi

    # Sonames provided by the Ubuntu base image (libc6 and the loader) are
    # never staged: /opt/lib is on the image LD_LIBRARY_PATH and must not
    # shadow the base runtime. Everything else in the closure is required.
    base_libs='^(ld-linux|libc\.so|libm\.so|libpthread|libdl|librt|libresolv|libgcc_s\.so)'

    # Walk the NEEDED closure transitively, starting with jujud's own
    # entries: every non-base library the binary and its staged
    # dependencies require must resolve from the multiarch directories.
    pending=$(readelf -d "${jujud_bin}" | sed -n 's/.*Shared library: \[\([^]]*\)\]/\1/p')
    seen=""
    staged=0
    rm -rf "${lib_dir}"
    mkdir -p "${lib_dir}"
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
        lib_path=""
        for dir in ${lib_dirs}; do
            if [ -e "${dir}/${soname}" ]; then
                lib_path="${dir}/${soname}"
                break
            fi
        done
        if [ -z "${lib_path}" ]; then
            echo "operator image staging: ${soname} (needed by the ${platform} jujud closure) was not found under ${lib_dirs};" >&2
            echo "install the arch-qualified packages: make install-dqlite-dependencies DQLITE_CROSS_ARCHES=<deb arch>" >&2
            exit 1
        fi
        real_path=$(readlink -f "${lib_path}")
        real_name=$(basename "${real_path}")
        cp -L "${real_path}" "${lib_dir}/${real_name}"
        if [ "${soname}" != "${real_name}" ]; then
            ln -sf "${real_name}" "${lib_dir}/${soname}"
        fi
        staged=$((staged + 1))
        pending="${pending}
$(readelf -d "${real_path}" | sed -n 's/.*Shared library: \[\([^]]*\)\]/\1/p')"
    done
    if [ "${staged}" -eq 0 ]; then
        echo "operator image staging: no non-base dynamic libraries found for ${jujud_bin};" >&2
        echo "the jujud controller binary must be dynamically linked" >&2
        exit 1
    fi
}

# stage_operator_image_libs stages the runtime shared-library closure of the
# dynamically linked jujud binary into the per-platform Docker context lib/
# directory. The Ubuntu image base provides the C runtime and the dynamic
# loader, so only the non-base closure required by the binary is staged;
# a binary whose closure cannot be resolved fails here instead of producing
# an image that can only fail at startup.
#
# The native closure is derived from the build host's ldd output. Host ldd
# is never run against a foreign-architecture ELF: a cross build's closure
# is derived from readelf and the Debian multiarch packages instead (see
# stage_foreign_operator_image_libs).
stage_operator_image_libs() {
    platform="${1:-}"
    os=$(echo "$platform" | cut -d/ -f1)
    arch=$(echo "$platform" | cut -d/ -f2)
    platform_dir="${BUILD_DIR}/${os}_${arch}"
    jujud_bin="${platform_dir}/bin/jujud"
    lib_dir=$(operator_image_lib_dir "$platform")
    host_arch=$(go env GOARCH)

    if [ ! -f "${jujud_bin}" ]; then
        echo "operator image staging: ${jujud_bin} not found; run 'make image-check' first" >&2
        exit 1
    fi
    if [ "${arch}" != "${host_arch}" ]; then
        # Foreign-architecture binary: the closure comes from readelf and
        # the target architecture's multiarch packages, never host ldd.
        stage_foreign_operator_image_libs "${platform}"
        return
    fi
    if ! closure=$(ldd "${jujud_bin}" 2>/dev/null); then
        echo "operator image staging: cannot resolve ${jujud_bin} with ldd" >&2
        exit 1
    fi
    if echo "${closure}" | grep -q "not found"; then
        echo "operator image staging: ${jujud_bin} has unresolved libraries:" >&2
        echo "${closure}" | grep "not found" >&2
        echo "Run 'make install-dependencies' to install the Dqlite runtime libraries (ppa:dqlite/dev), then retry." >&2
        exit 1
    fi

    rm -rf "${lib_dir}"
    mkdir -p "${lib_dir}"
    # Sonames provided by the Ubuntu base image (libc6 and the loader) are
    # never staged: /opt/lib is on the image LD_LIBRARY_PATH and must not
    # shadow the base runtime. Everything else in the closure is required.
    base_libs='^(ld-linux|libc\.so|libm\.so|libpthread|libdl|librt|libresolv|libgcc_s\.so)'
    staged=0
    for lib_path in $(echo "${closure}" | awk '/=>/ {print $3}'); do
        lib_soname=$(basename "${lib_path}")
        if echo "${lib_soname}" | grep -Eq "${base_libs}"; then
            continue
        fi
        real_path=$(readlink -f "${lib_path}")
        real_name=$(basename "${real_path}")
        cp -L "${real_path}" "${lib_dir}/${real_name}"
        if [ "${lib_soname}" != "${real_name}" ]; then
            ln -sf "${real_name}" "${lib_dir}/${lib_soname}"
        fi
        staged=$((staged + 1))
    done
    if [ "${staged}" -eq 0 ]; then
        echo "operator image staging: no non-base dynamic libraries found for ${jujud_bin};" >&2
        echo "the jujud controller binary must be dynamically linked" >&2
        exit 1
    fi
}

# require_operator_image_libs enforces the pre-staged library closure for
# image builds that do not build jujud from source (OPERATOR_IMAGE_BUILD_SRC
# set to false). The closure is an input owned by the CI payload (QA
# source-build closure or release snap extraction); it is never recomputed
# from or overwritten with host libraries here.
require_operator_image_libs() {
    platform="${1:-}"
    lib_dir=$(operator_image_lib_dir "$platform")
    if [ ! -d "${lib_dir}" ]; then
        echo "operator image build: missing pre-staged jujud library closure at ${lib_dir};" >&2
        echo "provide it as part of the build payload (source-build closure or snap extraction)" >&2
        exit 1
    fi
}


# build_push_operator_image is responsible for doing the heavy lifting when it
# comes time to build the Juju oci operator image. This function can also build
# the operator image for multiple architectures at once. Takes 3 arguments that
# describe one or more platforms to build for, whether to push the image, and
# where the jujud binary comes from.
# - $1 space seperated list of os/arch to build the image for. Follow the GO
#   idiom for naming. Example "linux/amd64 linux/arm64". The only supported OS
#   is linux at the moment. If no argument is provided defaults to GOOS & GOARCH
# - $2 true or false value on if the resultant image(s) should be pushed to the
#   registry
# - $3 true or false value on whether the jujud binary and its runtime library
#   closure come from a local source build (true, the default) or from a
#   pre-staged build payload (false, the CI contract)
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

    build_from_src=${3:-"true"}

    # Stage (or require) the jujud runtime library closure for every image
    # platform, per the dynamic-image-layout contract in caas/Dockerfile.
    for platform in $build_multi_osarch; do
        if [ "${build_from_src}" = "true" ]; then
            stage_operator_image_libs "${platform}"
        else
            require_operator_image_libs "${platform}"
        fi
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

# check_dqlite_cross_stale clears leftover distro (1.16-line) Dqlite
# packages for cross-build architectures. An early revision of the
# DQLITE_CROSS_ARCHES provisioning resolved the arch-qualified
# libdqlite-dev meta name to the distro's older real dev package; its
# half-completed transactions left libdqlite0:<arch> installed and a
# stuck want-install selection for libdqlite-dev:<arch>. The leftover
# owns the same plain sonames as the PPA series package the flow
# installs, so every later apt operation on the host fails deep inside
# dpkg (file overwrite or unmet dependencies) instead of at the
# provisioning step that caused it. The removal is loud and scoped to
# distro Dqlite packages for foreign architectures only; PPA series
# packages and native packages are never touched.
check_dqlite_cross_stale() {
    for arch in $(dpkg --print-foreign-architectures); do
        for name in libdqlite-dev libdqlite0; do
            st=$(dpkg-query -W -f='${db:Status-Abbrev}' "$name:$arch" 2>/dev/null | tr -d ' ') || st=""
            case "$st" in
                in)
                    echo "Clearing stuck install selection for $name:$arch (leftover from an earlier failed cross-provisioning run)" >&2
                    echo "$name:$arch deinstall" | sudo dpkg --set-selections
                    ;;
                i*)
                    echo "Removing leftover distro Dqlite package $name:$arch (state '$st') from an earlier failed cross-provisioning run" >&2
                    sudo dpkg --remove "$name:$arch"
                    ;;
            esac
        done
    done
}

# apt_update runs 'apt-get update' for the provisioning targets. It first
# clears any leftover distro Dqlite packages for cross architectures (see
# check_dqlite_cross_stale): while such a leftover exists, every apt
# operation on the host fails with unmet dependencies, so the cleanup must
# precede both the update and any later install. When foreign
# architectures are registered with dpkg, every repository whose
# Release file advertises them is asked for their indexes, and
# repositories that do not actually serve them report 404s; apt still
# uses the indexes that did fetch. Ubuntu's primary archive and most of
# its mirrors advertise every architecture but only serve amd64 and
# i386, so such failures are expected on a cross-build host and are
# tolerated with a note. Without foreign architectures registered the
# update stays strict.
apt_update() {
    check_dqlite_cross_stale
    if [ -z "$(dpkg --print-foreign-architectures)" ]; then
        sudo apt-get update
        return
    fi
    if ! sudo apt-get update; then
        echo "NOTE: 'apt-get update' could not fetch some indexes (see the errors above)." >&2
        echo "Foreign architectures are registered ($(dpkg --print-foreign-architectures | tr '\n' ' '));" >&2
        echo "repositories that do not serve them report 404s, which is expected. apt" >&2
        echo "continues with the indexes that were fetched. Restrict such repositories" >&2
        echo "with an 'arch=' option on their source entries to silence the errors." >&2
    fi
}

# write_dqlite_cross_apt_sources writes an apt sources file giving each
# requested cross-build architecture a package index it can actually
# fetch. Ubuntu splits its archive by architecture: amd64 and i386 live
# on the primary archive (archive.ubuntu.com, security.ubuntu.com) while
# every other architecture lives on ports.ubuntu.com. Mirrors of the
# primary archive advertise all architectures in their Release files but
# generally serve only amd64/i386, so a dpkg-registered foreign
# architecture needs index entries of its own. Each entry is restricted
# to a single architecture, so it never changes which indexes the host's
# own sources fetch.
write_dqlite_cross_apt_sources() {
    arches="${1:-}"
    sources_file="${2:-/etc/apt/sources.list.d/juju-dqlite-cross-arch.list}"
    if [ -z "${arches}" ]; then
        echo "write_dqlite_cross_apt_sources: no architectures given" >&2
        return 1
    fi
    . /etc/os-release
    if [ -z "${UBUNTU_CODENAME:-}" ]; then
        echo "write_dqlite_cross_apt_sources: /etc/os-release has no UBUNTU_CODENAME; only Ubuntu hosts are supported" >&2
        return 1
    fi
    tmp_file=$(mktemp)
    {
        echo "# Managed by juju 'make install-dqlite-dependencies' (DQLITE_CROSS_ARCHES)."
        echo "# Package indexes for cross-building the dynamic jujud. Undo with:"
        echo "#   sudo rm ${sources_file} && sudo dpkg --remove-architecture <arch>"
        for arch in ${arches}; do
            case "${arch}" in
                amd64|i386)
                    mirror="https://archive.ubuntu.com/ubuntu"
                    security_mirror="https://security.ubuntu.com/ubuntu"
                    ;;
                *)
                    mirror="https://ports.ubuntu.com/ubuntu-ports"
                    security_mirror="https://ports.ubuntu.com/ubuntu-ports"
                    ;;
            esac
            echo "deb [arch=${arch}] ${mirror} ${UBUNTU_CODENAME} main universe"
            echo "deb [arch=${arch}] ${mirror} ${UBUNTU_CODENAME}-updates main universe"
            echo "deb [arch=${arch}] ${security_mirror} ${UBUNTU_CODENAME}-security main universe"
        done
    } >"${tmp_file}"
    sudo tee "${sources_file}" <"${tmp_file}" >/dev/null
    rm -f "${tmp_file}"
    echo "Wrote ${sources_file} (cross-architecture package indexes for: ${arches})"
}
