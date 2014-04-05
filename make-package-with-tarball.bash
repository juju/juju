#!/bin/bash
#
# Create source and binary packages using a source package branch and
# a release tarball.

set -e

HERE=$(pwd)
SCRIPT_DIR=$(cd $(dirname "${BASH_SOURCE[0]}") && pwd )

DEFAULT_JUJUDB_PACKAGING_BRANCH="lp:~juju-qa/juju-core/devel-packaging"
DEFAULT_MONGODB_PACKAGING_BRANCH="lp:~juju-qa/juju-core/devel-mongodb-packaging"
TESTING_SERIES="trusty"
EXTRA_RELEASES="saucy:13.10 precise:12.04"


usage() {
    echo "usage: $0 <SERIES> tarball 'name-email' [bug-number ...]"
    echo "  SERIES: The series name which selects the packaging branch."
    echo "  tarball: The path to the juju-core tarball."
    echo "  name-email: The 'name <email>' string used in the changelog."
    echo "  bug-number: Zero or more Lp bug numbers"
    exit 1
}


check_deps() {
    echo "Phase 0: Checking requirements."
    has_deps=1
    which dch || has_deps=0
    which bzr || has_deps=0
    bzr plugins | grep builddeb || has_deps=0
    if [[ $has_deps == 0 ]]; then
        echo "Install devscripts, bzr, and bzr-builddeb"
        exit 2
    fi
}


make_source_package_branch() {
    echo "Phase 1: Updating the source package branch."
    if [[ $SERIES == "trusty" ]]; then
        PACKAGING_BRANCH=$DEFAULT_JUJUDB_PACKAGING_BRANCH
    else
        PACKAGING_BRANCH=$DEFAULT_MONGODB_PACKAGING_BRANCH
    fi
    echo "Using $PACKAGING_BRANCH"
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
        distro=$SERIES
    elif [[ $PURPOSE == "devel" ]]; then
        message="New upstream devel release."
        distro=$SERIES
    else
        message="New upstream release candidate."
        distro="UNRELEASED"
    fi
    if [[ $BUGS != "" ]]; then
        message="$message (LP: $BUGS)"
    fi
    DEBEMAIL=$DEBEMAIL dch --newversion $UBUNTU_VERSION -D $distro "$message"
    bzr ci -m "$message"
    bzr tag $UBUNTU_VERSION
}


make_source_package() {
    echo "Phase 2: Creating the source package for ubuntu devel."
    cd $PACKAGING_DIR
    bzr bd -S --build-dir=$BUILD_DIR
    echo "The source package can be uploaded:"
    echo "  cd $TMP_DIR"
    echo "  dput $PPA juju-core_${UBUNTU_VERSION}_source.changes"
}


make_binary_package() {
    package_version=$1
    package_series=$2
    echo "Phase 3: Creating the binary package for $package_series."
    cd $PACKAGING_DIR
    bzr bd --build-dir=$BUILD_DIR -- -uc -us
    new_package=$(ls ${TMP_DIR}/juju-core_${package_version}*.deb)
    echo "Made $new_package"
    ln -s $new_package ${TMP_DIR}/new-${package_series}.deb
    echo "linked $new_package to ${TMP_DIR}/new-${package_series}.deb"
}


update_source_package_branch() {
    package_version=$1
    package_series=$2
    echo "Phase 4: Backport the source package branch to $package_series."
    cd $PACKAGING_DIR
    message="New upstream release candidate."
    distro="UNRELEASED"
    DEBEMAIL=$DEBEMAIL dch -b --newversion $package_version \
        -D $distro "$message"
    bzr ci -m "$message"
    bzr tag $package_version
}


make_binary_packages() {
    make_binary_package $UBUNTU_VERSION $SERIES
    # Make extra packages for supported series.
    for series_release in $EXTRA_RELEASES; do
        this_series=$(echo "$series_release" | cut -d ':' -f1)
        this_release=$(echo "$series_release" | cut -d ':' -f2)
        package_version="${VERSION}-0ubuntu1~ubuntu${this_release}.${PPATCH}"
        update_source_package_branch $package_version $this_series
        make_binary_package $package_version $this_series
    done
    NEW_PACKAGES=$(ls ${TMP_DIR}/new-*.deb)
    echo "New packages for testing: $NEW_PACKAGES"
}


PPATCH="1"
while getopts ":p:" o; do
    case "${o}" in
        p)
            PPATCH=${OPTARG}
            echo "Setting package patch to $PPATCH"
            ;;
        *)
            usage
            ;;
    esac
done
shift $((OPTIND - 1))

test $# -ge 3 || usage

SERIES=$1
TARBALL=$(readlink -f $2)
DEBEMAIL=$3
shift; shift; shift
BUGS=$(echo "$@" | sed  -e 's/ /, /g; s/\([0-9]\+\)/#\1/g;')
if [[ ! -f "$TARBALL" ]]; then
    echo "Tarball not found."
    usage
fi
check_deps

VERSION=$(basename $TARBALL .tar.gz | cut -d '_' -f2)
if [[ $SERIES == "testing" ]]; then
    PURPOSE="testing"
    SERIES=$TESTING_SERIES
elif [[ $VERSION =~ ^1.(18|20|22).*$ ]]; then
    PURPOSE="stable"
else
    PURPOSE="devel"
fi
RELEASE=$(cat $SCRIPT_DIR/supported-releases.txt |
    grep $SERIES | cut -d ' ' -f 1)
UBUNTU_VERSION="${VERSION}-0ubuntu${RELEASE}.${PPATCH}~juju1"

TMP_DIR=$(mktemp -d --tmpdir=$HERE)
PACKAGING_DIR="$TMP_DIR/packaging"
BUILD_DIR="$TMP_DIR/build"

make_source_package_branch
if [[ $PURPOSE == "testing" ]]; then
    make_binary_packages
else
    make_source_package
fi
echo "You can delete this directory when you are done:"
echo "  $TMP_DIR"
