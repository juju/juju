#!/bin/bash
#
# Create source and binary packages using a source package branch and
# a release tarball.

set -e

HERE=$(pwd)
TMP_DIR=$(mktemp -d --tmpdir=$HERE)
PACKAGING_DIR="$TMP_DIR/juju-packaging"
BUILD_DIR="$TMP_DIR/juju-build"
DEFAULT_STABLE_PACKAGING_BRANCH="lp:ubuntu/juju-core"
DEFAULT_DEVEL_PACKAGING_BRANCH="lp:~juju-qa/juju-core/devel-packaging"
DEVEL_SERIES="trusty"
SERIES_VERSION=""
LTS_VERSTION="~ubuntu12.04"


usage() {
    echo "usage: $0 <PURPOSE> tarball 'name-email'"
    echo "  PURPOSE: stable, devel, or testing,"
    echo "     which selects the packaging branch."
    echo "  tarball: The path to the juju-core tarball."
    echo "  name-email: The 'name <email>' string used in the charngelog."
    exit 1
}


check_deps() {
    echo "Phase 0: Checking requirements."
    has_deps=1
    which dch || has_deps=0
    which bzr || has_deps=0
    bzr plugins | grep builddeb || has_deps=0
    if [[ $has_deps == 0 ]]; then
        echo "Install devscripts, bzr, and bzr-builder"
        exit 2
    fi
}


make_soure_package_branch() {
    echo "Phase 1: Updating the source package branch."
    bzr branch $PACKAGING_BRANCH $PACKAGING_DIR
    cd $PACKAGING_DIR
    bzr import-upstream $VERSION $TARBALL
    bzr merge . -r upstream-${VERSION}
    if [[ $PURPOSE == "stable" ]]; then
        if [[ $VERSION =~ .*\.0$ ]]; then
            message="New upstream stable release."
        else
            message="New upstream point release."
        fi
        distro=$DEVEL_SERIES
    elif [[ $PURPOSE == "devel" ]]; then
        message="New upstream devel release."
        distro=$DEVEL_SERIES
    else
        messasge="New upstream release candidate."
        distro="UNRELEASED"
    fi
    DEBEMAIL=$DEBEMAIL dch --newversion $UBUNTU_VERSION -D $distro "$message"
    bzr ci -m "$messasge"
    bzr tag UBUNTU_VERSION
}


make_source_package() {
    echo "Phase 2: Creating the source package."
    cd $PACKAGING_DIR
    bzr bd -S --build-dir=$BUILD_DIR
    cd $BUILD_DIR
    echo "The source package can be uploaded:"
    echo "  cd $TMP_DIR"
    echo "  dput $PPA juju-core_${UBUNTU_VERSION}_source.changes"
}


make_binary_package() {
    echo "Phase 3: Creating the binary package."
    cd $PACKAGING_DIR
    bzr bd --build-dir=$BUILD_DIR
    package=$(ls ${TMP_DIR}/juju-core_*.deb)
    echo "The binary package can be installed:"
    echo "  sudo dpkg -i $package"

}


test $# -eq 3 || usage

PURPOSE=$1
if [[ $PURPOSE == "stable" ]]; then
    PACKAGING_BRANCH=$DEFAULT_STABLE_PACKAGING_BRANCH
    PPA="ppa:juju-packaging/stable"
elif [[ $PURPOSE == "devel" || $PURPOSE == "testing" ]]; then
    PACKAGING_BRANCH=$DEFAULT_DEVEL_PACKAGING_BRANCH
    PPA="ppa:juju-packaging/devel"
    if [[ $PURPOSE == "testing" ]]; then
        SERIES_VERSION=$LTS_VERSION
    fi
else
    usage
fi

TARBALL=$HERE/$2
if [[ ! -f "$TARBALL" ]]; then
    echo "Tarball not found."
    usage
fi
VERSION=$(basename -s .tar.gz $TARBALL | cut -d '_' -f2)
UBUNTU_VERSION="${VERSION}-0ubuntu1${SERIES_VERSION}"

DEBEMAIL=$3

check_deps
make_soure_package_branch
if [[ $PURPOSE == "testing" ]]; then
    make_binary_package
else
    make_source_package
fi
echo "You can delete this directories when you are done:"
echo "  $TMP_DIR"
