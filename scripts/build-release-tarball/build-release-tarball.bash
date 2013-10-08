#!/usr/bin/env bash -e

# if someone invokes this with bash 
set -e

unset GOPATH

# build release tarball from a bzr branch 

usage() {
	echo usage: $0 TAG
	exit 2
}

# bzr-checkout $SOURCE_URL $TAG $TARGET_DIR
bzr-checkout() {
	echo "cloning $1 at revision $2"
	bzr checkout --lightweight -r $2 $1 ${WORK}/src/$3
}

test $# -eq 1 ||  usage 
TAG=$1
TMP_DIR=$(mktemp -d)
mkdir $TMP_DIR/RELEASE
WORK=$TMP_DIR/RELEASE

# populate top level dirs
mkdir -p $WORK/src/launchpad.net $WORK/src/labix.org/v2

# Checkout juju manuallly.
bzr-checkout lp:juju-core $TAG launchpad.net/juju-core

# Fetch dependencies; restore juju-core to the proper revision; set the deps
# to the proper revision.
GOPATH=$WORK go get -v -d launchpad.net/juju-core/...
(cd "${WORK}/src/launchpad.net/juju-core/" && bzr revert -r $TAG)
GOPATH=$WORK godeps -u "${WORK}/src/launchpad.net/juju-core/dependencies.tsv"

# smoke test
GOPATH=$WORK go build -v launchpad.net/juju-core/...

# Fetch the version and set the release name.
VERSION=$(sed -n 's/^const version = "\(.*\)"/\1/p' $WORK/src/launchpad.net/juju-core/version/version.go)
mv $WORK $TMP_DIR/juju-core_${VERSION}/

# Tar it up.
TARFILE=`pwd`/juju-core_${VERSION}.tar.gz
cd $TMP_DIR
tar cfz $TARFILE --exclude .hg --exclude .git --exclude .bzr juju-core_${VERSION}

echo "release tarball: ${TARFILE}"
