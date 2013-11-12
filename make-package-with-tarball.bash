#!/bin/bash
#
# Create source and binary packages using a source package branch and
# a release tarball.

set -e

HERE=$(pwd)
PACKAGING_DIR="$HERE/juju-packaging"
BUILD_DIR="$HERE/juju-build"
DEFAULT_STABLE_PACKAGING_BRANCH="lp:ubuntu/juju-core"
# XXX sinzui 2013-11-11: There is something wrong with this branch. Consider
# forking lp:ubuntu/juju-core.
DEFAULT_DEVEL_PACKAGING_BRANCH="lp:~juju-qa/juju-core/devel-packaging"
LTS_SERIES="precise"


usage() {
    echo "usage: $0 <PURPOSE> tarball"
    echo "  PURPOSE: stable, devel, or testing,"
    echo "     which selects the packaging branch."
    echo "  tarball: The path to the juju-core tarball."
    exit 1
}


check_deps() {
    echo "Phase 0: Checking requirements."
    has_deps=1
    which devscripts || has_deps=0
    which bzr || has_deps=0
    bzr plugins | grep bzr-builddeb || has_deps=0
    if [[ $has_deps == 0 ]]; then
        echo "Install devscripts, bzr, and bzr-builder"
        exit 2
    fi
}


make_soure_package_branch() {
    bzr branch $PACKAGING_BRANCH $PACKAGING_DIR
    cd juju-packaging/
    bzr import-upstream $VERSION $TARBALL
    bzr merge . -r upstream-${VERSION}
    if [[ $PURPOSE == "stable" ]]; do
        if [[ $VERSION =~ .*\.0$ ]]; do
            message="New upstream stable release."
        else
            message="New upstream point release."
        fi
        dch --newversion $UBUNTU_VERSION -D $LTS_SERIES "$message"
    elif [[ $PURPOSE == "devel" ]]; do
        message="New upstream devel release."
        dch --newversion $UBUNTU_VERSION -D $LTS_SERIES "$message"
    else
        messasge="New upstream release candidate."
        dch --newversion $UBUNTU_VERSION "$message"
    done
    bzr ci -m "$messasge"
    bzr tag UBUNTU_VERSION
}


make_binary_package() {
    cd PACKAGING_DIR
    bzr bd --build-dir=$HERE/juju-build
}


make_source_package() {
    cd PACKAGING_DIR
    bzr bd -S --build-dir=$HERE/juju-build
    cd $BUILD_DIR
    dput $PPA juju-core_${UBUNTU_VERSION}_source.changes
}


test $# -eq 2 || usage

PURPOSE=$1
if [[ $PURPOSE != "stable" ]]; then
    PACKAGING_BRANCH=$DEFAULT_STABLE_PACKAGING_BRANCH
    PPA="ppa:juju-packaging/stable"
elif [[ $PURPOSE != "devel" || $PURPOSE != "testing" ]]; then
    PACKAGING_BRANCH=$DEFAULT_DEVEL_PACKAGING_BRANCH
    PPA="ppa:juju-packaging/devel"
else
    usage
fi

TARBALL=$2
if [[ ! -f "$TARBALL" ]]; then
    echo "Tarball not found."
    usage
fi
VERSION=$(basename -s .tar.gz $TARBALL | cut -d '_' -f2)
UBUNTU_VERSION="${VERSION}-0ubuntu1"

check_deps
mkdir $PACKAGING_DIR
mkdir $BUILD_DIR
make_soure_package_branch
make_binary_package



