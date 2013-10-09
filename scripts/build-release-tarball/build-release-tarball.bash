#!/usr/bin/env bash -e

# if someone invokes this with bash
set -e

unset GOPATH

# build release tarball from a bzr branch

usage() {
	echo usage: $0 TAG
	exit 2
}

test $# -eq 1 ||  usage
TAG=$1
TMP_DIR=$(mktemp -d)
mkdir $TMP_DIR/RELEASE
WORK=$TMP_DIR/RELEASE

echo "Getting juju-core and all its dependencies."
GOPATH=$WORK go get -v -d launchpad.net/juju-core/...

echo "Setting juju-core tree to $TAG."
(cd "${WORK}/src/launchpad.net/juju-core/" && bzr revert -r $TAG)

echo "Updating juju-core dependencies to the required versions."
GOPATH=$WORK godeps -u "${WORK}/src/launchpad.net/juju-core/dependencies.tsv"

# Smoke test
GOPATH=$WORK go build -v launchpad.net/juju-core/...

# Change the generic release to the proper juju-core version.
VERSION=$(sed -n 's/^const version = "\(.*\)"/\1/p' \
    $WORK/src/launchpad.net/juju-core/version/version.go)
mv $WORK $TMP_DIR/juju-core_${VERSION}/

# Tar it up.
TARFILE=`pwd`/juju-core_${VERSION}.tar.gz
cd $TMP_DIR
tar cfz $TARFILE --exclude .hg --exclude .git --exclude .bzr juju-core_${VERSION}

echo "release tarball: ${TARFILE}"
