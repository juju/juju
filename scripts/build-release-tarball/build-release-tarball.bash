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
mkdir -p $WORK/src/launchpad.net $WORK/src/labix.org/v2 $WORK/src/code.google.com/p/go.{net,crypto} 

# checkout juju (manually, because we're redefining $WORK later on
bzr-checkout lp:juju-core $TAG launchpad.net/juju-core

# fetch the version
VERSION=$(sed -n 's/^const version = "\(.*\)"/\1/p' $WORK/src/launchpad.net/juju-core/version/version.go)

# fixup paths for tarball
mkdir $WORK/juju-core_${VERSION}
mv $WORK/src $WORK/juju-core_${VERSION}/
WORK=$WORK/juju-core_${VERSION}

# fetch dependencies
hg-checkout https://code.google.com/p/go.net tip code.google.com/p/go.net
hg-checkout https://code.google.com/p/go.crypto tip code.google.com/p/go.crypto

declare -A deps_revno
declare -A deps_vcs
while read -r lib vcs revision revno; do
    if [[ $vcs == "bzr" ]]; then
        deps_revno+=(["$lib"]="$revno")
        deps_vcs+=(["$lib"]="$vcs")
    fi
done < "${WORK}/src/launchpad.net/juju-core/dependencies.tsv"

for lib in "${!deps_revno[@]}"; do
    revno=${deps_revno["$lib"]}
    vcs=${deps_vcs["$lib"]}
    location=$(
        echo $lib |
        sed -e 's,launchpad.net/,lp:,; s,labix.org/v2/mgo,lp:mgo/v2,;')
    # gnuflag has GhostRevisionsHaveNoRevno error. stacking error?
    if [[ $lib == 'launchpad.net/gnuflag' ]]; then
        location='lp:~gophers/gnuflag/trunk'
    fi
    bzr-checkout $location $revno $lib
done

# smoke test
GOPATH=$WORK go build -v launchpad.net/juju-core/...

# tar it up
TARFILE=`pwd`/juju-core_${VERSION}.tar.gz
cd $WORK/..
tar cfz $TARFILE --exclude .hg --exclude .git --exclude .bzr juju-core_${VERSION}

echo "release tarball: ${TARFILE}"
