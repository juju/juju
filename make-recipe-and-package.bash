#!/bin/bash
# Create a bzr-builder recipe and build it.
#
# The package built by this script can be used for testing to verify
# the release candidate is viable. The recipe can be added to launchpad
# to build packages (after the RC has proven to be viable).
set -eu

HERE=$(pwd)
DEFAULT_JUJU_CORE="lp:juju-core"
DEFAULT_PACKAGING="lp:~ce-orange-squad/juju-core/unstable-packaging"
LTS_SERIES="precise"


usage() {
    echo "usage: $0 REVNO [JUJU_CORE_BRANCH [PACKAGING_BRANCH]]"
    echo "  REVNO: The juju-core revno to build"
    echo "  JUJU_CORE_BRANCH: The juju-core branch; defaults to ${DEFAULT_JUJU_CORE}"
    echo "  PACKAGING_BRANCH: The packaging branch; defaults to ${DEFAULT_PACKAGING}"
    exit 1
}


check_deps() {
    echo "Phase 0: Checking requirements."
    has_deps=1
    which dh || has_deps=0
    which bzr || has_deps=0
    bzr plugins | grep bzr-builder || has_deps=0
    if [[ $has_deps == 0 ]]; then
        echo "Install debhelper, bzr, and bzr-builder"
        exit 2
    fi
}


create_recipe() {
    echo "Phase 1: Creating a recipe for juju-core r${REVNO}."
    echo "Retrieving dependencies.tsv from $JUJU_CORE_BRANCH r$REVNO"
    VERSION=$(bzr cat -q -d $JUJU_CORE_BRANCH -r $REVNO version/version.go |
        sed -n 's/^const version = "\(.*\)"/\1/p')
    DEPENDENCIES=$(bzr cat -d $JUJU_CORE_BRANCH -r $REVNO dependencies.tsv)
    BASE="\
# bzr-builder format 0.3 deb-version ${VERSION}+${REVNO}-0
${PACKAGING_BRANCH}
nest juju-core ${JUJU_CORE_BRANCH} src/launchpad.net/juju-core revno:${REVNO}
"
    NESTED=$(
        echo "${DEPENDENCIES}" |
        sed -e '/^code/d;' \
            -e 's,^\(.*\)/\([^/]*\)\tbzr.*\t\([0-9]*\)$,nest \2 lp:\2 src/\1/\2 revno:\3,;' \
            -e 's,lp:mgo,lp:mgo/v2,' |
        sort)
    RECIPE="${BASE}${NESTED}"
    echo "${RECIPE}" > $RECIPE_NAME
    echo "Created ${RECIPE_NAME}"
}


create_source_package() {
    echo "Phase 2: Creating a source package."
    mkdir -p "${TESTING_DIR}"
    bzr dailydeb --allow-fallback-to-native --append-version ~$LTS_SERIES \
        $RECIPE_NAME $TESTING_DIR
}


create_binary_package() {
    echo "Phase 3: creating a binary package."
    cd ${TESTING_DIR}
    tar zxf *.tar.gz
    cd juju-core-$VERSION+$REVNO/
    fakeroot debian/rules binary
    PACKAGES=$(ls ${TESTING_DIR}/*.deb)
    echo "Created $PACKAGES"
}


test $# -ge 1 ||  usage
REVNO=$1
JUJU_CORE_BRANCH=${2:-$DEFAULT_JUJU_CORE}
PACKAGING_BRANCH=${3:-$DEFAULT_PACKAGING}

RECIPE_NAME="juju-core-r${REVNO}.recipe"
TESTING_DIR="${HERE}/testing-juju-core-r${REVNO}"

check_deps
create_recipe
create_source_package
create_binary_package
