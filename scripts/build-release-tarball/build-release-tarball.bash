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

# hg-checkout $SOURCE_URL $TAG $TARGET_DIR
hg-checkout() {
	echo "cloning $1 at revision $2"
	hg clone -q -r $2 $1 ${WORK}/src/$3
}

# git-checkout $SOURCE_URL $TARGET_DIR
git-checkout() {
	echo "cloning $1"
	git clone -q $1 ${WORK}/src/$2
}

test $# -eq 1 ||  usage 
TAG=$1
WORK=$(mktemp -d)

# populate top level dirs
mkdir -p $WORK/src/launchpad.net $WORK/src/labix.org/v2

# checkout juju (manually, because we're redefining $WORK later on
bzr-checkout lp:juju-core $TAG launchpad.net/juju-core

# fetch the version
VERSION=$(sed -n 's/^const version = "\(.*\)"/\1/p' $WORK/src/launchpad.net/juju-core/version/version.go)

# fixup paths for tarball
mkdir $WORK/juju-core_${VERSION}
mv $WORK/src $WORK/juju-core_${VERSION}/
WORK=$WORK/juju-core_${VERSION}

# Fetch dependencies; restore juju-core to the proper revision; set the deps
# to the properrevision.
GOPATH=$WORK go get -v -d launchpad.net/juju-core/...
(cd "${WORK}/src/launchpad.net/juju-core/" && bzr revert -r $TAG)
GOPATH=$WORK godeps -u "${WORK}/src/launchpad.net/juju-core/dependencies.tsv"

# smoke test
GOPATH=$WORK go build -v launchpad.net/juju-core/...

# tar it up
TARFILE=`pwd`/juju-core_${VERSION}.tar.gz
cd $WORK/..
tar cfz $TARFILE --exclude .hg --exclude .git --exclude .bzr juju-core_${VERSION}

echo "release tarball: ${TARFILE}"
