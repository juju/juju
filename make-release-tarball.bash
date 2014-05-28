#!/bin/bash

# if someone invokes this with bash
set -e

unset GOPATH
unset GOBIN

# build release tarball from a bzr branch
DEFAULT_JUJU_CORE="lp:juju-core"


usage() {
    echo "usage: $0 <BZR_REVNO|GIT_REVISION> [BZR_JUJU_CORE_BRANCH|GIT_REPO]"
    echo "  BZR_REVNO: The juju-core bzr revno to build"
    echo "  GIT_REVISION: The juju-core git revision or branch to build,"
    echo "  BZR_JUJU_CORE_BRANCH: The juju-core bzr branch; defaults to ${DEFAULT_JUJU_CORE}"
    echo "  GIT_REPO: The juju-core git repo"
    exit 1
}


check_deps() {
    echo "Phase 0: Checking requirements."
    has_deps=1
    which bzr || has_deps=0
    which git || has_deps=0
    which hg || has_deps=0
    which go || has_deps=0
    if [[ $has_deps == 0 ]]; then
        echo "Install bzr, hg, git, and golang."
        exit 2
    fi
}


test $# -ge 1 ||  usage
check_deps
REVNO=$1
JUJU_CORE_BRANCH=${2:-$DEFAULT_JUJU_CORE}
TMP_DIR=$(mktemp -d --tmpdir=$(pwd))
mkdir $TMP_DIR/RELEASE
WORK=$TMP_DIR/RELEASE

echo "Getting juju-core and all its dependencies."
GOPATH=$WORK go get -v -d launchpad.net/juju-core/... || \
    GOPATH=$WORK go get -v -d launchpad.net/juju-core/... || \
    GOPATH=$WORK go get -v -d launchpad.net/juju-core/...

echo "Setting juju-core tree to $JUJU_CORE_BRANCH $REVNO."
(cd "${WORK}/src/launchpad.net/juju-core/" &&
 bzr pull --no-aliases --remember --overwrite -r $REVNO $JUJU_CORE_BRANCH)

# Devs moved a package.
if [[ $JUJU_CORE_BRANCH == 'lp:juju-core/1.18' ]]; then
    echo "Moving deps to support 1.18 releases."
    GOPATH=$WORK go get -v github.com/errgo/errgo
    rm -rf $WORK/src/github.com/juju/errgo
fi

echo "Updating juju-core dependencies to the required versions."
GOPATH=$WORK go get -v launchpad.net/godeps
GODEPS=$WORK/bin/godeps
if [[ ! -f $GODEPS ]]; then
    echo "! Could not install godeps."
    exit 1
fi
GOPATH=$WORK $GODEPS -u "${WORK}/src/launchpad.net/juju-core/dependencies.tsv"
# Remove godeps.
rm -r $WORK/bin

# Smoke test
GOPATH=$WORK go build -v launchpad.net/juju-core/...

# Change the generic release to the proper juju-core version.
VERSION=$(sed -n 's/^const version = "\(.*\)"/\1/p' \
    $WORK/src/launchpad.net/juju-core/version/version.go)
mv $WORK $TMP_DIR/juju-core_${VERSION}/

# Tar it up.
TARFILE=$(pwd)/juju-core_${VERSION}.tar.gz
cd $TMP_DIR
tar cfz $TARFILE --exclude .hg --exclude .git --exclude .bzr juju-core_${VERSION}

echo "release tarball: ${TARFILE}"
