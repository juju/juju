#!/bin/bash
# Download Ubuntu juju packages that match the version under test.
set -eu

: ${LOCAL_JENKINS_URL=http://juju-ci.vapour.ws:8080}
ARTIFACTS_PATH=$WORKSPACE/artifacts

: ${SCRIPTS=$(readlink -f $(dirname $0))}
export PATH="$SCRIPTS:$PATH"

UBUNTU_ARCH="http://archive.ubuntu.com/ubuntu/pool/universe/j/juju-core/"
PORTS_ARCH="http://ports.ubuntu.com/pool/universe/j/juju-core/"
ALL_ARCHIVES="$UBUNTU_ARCH $PORTS_ARCH"

TOKEN="chiyo-sakaki-osaka-yomi-tomo"
TRUSTY_AMD64="certify-trusty-amd64"
TRUSTY_PPC64="certify-trusty-ppc64"
TRUSTY_I386="certify-trusty-i386"

set -x

usage() {
    echo "usage: $0 VERSION"
    echo "  VERSION: The juju package version to retrive."
    exit 1
}


check_deps() {
    echo "Phase 0: Checking requirements."
    has_deps=1
    which lftp || has_deps=0
    if [[ $has_deps == 0 ]]; then
        echo "Install lftp."
        exit 2
    fi
}


setup_workspace() {
    rm $WORKSPACE/* -rf
    mkdir -p $ARTIFACTS_PATH
    touch $ARTIFACTS_PATH/empty
}


retrieve_packages() {
    # Retrieve the $RELEASE packages that contain jujud,
    # or copy a locally built package.
    echo "Phase 1: Retrieving juju-core packages from archives"
    cd $WORKSPACE
    for archive in $ALL_ARCHIVES; do
        safe_archive=$(echo "$archive" | sed -e 's,//.*@,//,')
        echo "checking $safe_archive for $VERSION."
        lftp -c mirror -I "juju*${VERSION}*.deb" $archive;
    done
    if [ -d $WORKSPACE/juju-core ]; then
        found=$(find $WORKSPACE/juju-core/ -name "*deb")
        if [[ $found != "" ]]; then
            mv $WORKSPACE/juju-core/*deb ./
        fi
        rm -r $WORKSPACE/juju-core
    fi
}


start_series_arch_tests() {
    [[ $START_OTHER_TESTS == "false" ]] && return 0
    for job in $TRUSTY_AMD64 $TRUSTY_PPC64 $TRUSTY_I386; do
        curl -o /dev/null $LOCAL_JENKINS_URL/jobs/$job/build?token=$TOKEN
    done
}


START_OTHER_TESTS="false"
while [[ "${1-}" != "" && $1 =~ ^-.*  ]]; do
    case $1 in
        --start-other-tests)
            START_OTHER_TESTS="true"
            ;;
    esac
    shift
done

test $# -eq 1 || usage
VERSION=$1

check_deps
setup_workspace
retrieve_packages
start_series_arch_tests
