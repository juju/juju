#!/bin/bash
#
# Create source and binary packages using a source package branch and
# a release tarball.

set -e

HERE=$(pwd)
SCRIPT_DIR=$(cd $(dirname "${BASH_SOURCE[0]}") && pwd )

DEFAULT_JUJUDB_PACKAGING_BRANCH="lp:~juju-qa/juju-core/devel-packaging"
DEFAULT_MONGODB_PACKAGING_BRANCH="lp:~juju-qa/juju-core/devel-mongodb-packaging"


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
    if [[ $SERIES == "saucy" || $SERIES == "precise" ]]; then
        PACKAGING_BRANCH=$DEFAULT_MONGODB_PACKAGING_BRANCH
        
    else
        PACKAGING_BRANCH=$DEFAULT_JUJUDB_PACKAGING_BRANCH
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
    cd $HERE
}


make_source_package() {
    echo "Phase 2: Creating the source package."
    cd $PACKAGING_DIR
    if [[ $PURPOSE == "testing" ]]; then
        echo "The package is unsigned."
        bzr bd -S --build-dir=$BUILD_DIR -- -us -uc
    else
        echo "The package is signed."
        bzr bd -S --build-dir=$BUILD_DIR
    fi
    echo "The source package can be uploaded:"
    echo "  cd $TMP_DIR"
    echo "  dput $PPA juju-core_${UBUNTU_VERSION}_source.changes"
    cd $HERE
}


PPATCH="${PPATCH:-1}"
IS_TESTING="${IS_TESTING:-false}"
while getopts "p:t" o; do
    case "${o}" in
        p)
            PPATCH=${OPTARG}
            echo "Setting package patch to $PPATCH"
            ;;
        t)
            IS_TESTING="true"
            echo "Package will be for testing"
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
if [[ $IS_TESTING == "true" ]]; then
    PURPOSE="testing"
elif [[ $VERSION =~ ^1.(18|20|22).*$ ]]; then
    PURPOSE="stable"
else
    PURPOSE="devel"
fi
RELEASE=$(cat $SCRIPT_DIR/supported-releases.txt |
    grep $SERIES | cut -d ' ' -f 1)
UBUNTU_VERSION="${VERSION}-0ubuntu1~${RELEASE}.${PPATCH}~juju1"

TMP_DIR=$(mktemp -d --tmpdir=$HERE)
PACKAGING_DIR="$TMP_DIR/packaging"
BUILD_DIR="$TMP_DIR/build"

make_source_package_branch
make_source_package
echo "You can delete this directory when you are done:"
echo "  $TMP_DIR"
