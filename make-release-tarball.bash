#!/bin/bash

# if someone invokes this with bash
set -e

unset GOPATH
unset GOBIN

# build release tarball from a bzr or git branch
DEFAULT_BZR_JUJU_CORE="lp:juju-core"
DEFAULT_GIT_JUJU_CORE="https://github.com/juju/juju.git"

usage() {
    echo "usage: $0 <BZR_REVNO|GIT_REV> [BZR_BRANCH|GIT_REPO] [MERGE_REF] [MERGE_REPO] [MERGE_REV]"
    echo "  BZR_REVNO: The juju core bzr revno to build"
    echo "  GIT_REV: The juju core git revision or branch to build"
    echo "  BZR_BRANCH: The juju core bzr branch; defaults to $DEFAULT_BZR_JUJU_CORE"
    echo "  GIT_REPO: The juju core git repo; defaults to $DEFAULT_GIT_JUJU_CORE"
    echo "  MERGE_REF: The git branch or tag to merge"
    echo "  MERGE_REPO: The git repo that contains the MERGE_REF"
    echo "  MERGE_REV: The optional git specific commit to merge"
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

MERGE_REPO=""
MERGE_REF=""
MERGE_REV=""



if echo "$1" | grep -E '^[0-9-]*$'; then
    IS_BZR="true"
    REVNO=$1
    JUJU_CORE_BRANCH=${2:-$DEFAULT_BZR_JUJU_CORE}
    PACKAGE="launchpad.net/juju-core"
else
    IS_BZR="false"
    REVISION=$1
    JUJU_CORE_REPO=${2:-$DEFAULT_GIT_JUJU_CORE}
    PACKAGE="github.com/juju/juju"
    if [[ $# -ge 4 ]]; then
        MERGE_REF=$3
        MERGE_REPO=$4
        if [[ $# -eq 5 ]]; then
            MERGE_REV=$5
        fi
    fi
fi


HERE=$(pwd)
TMP_DIR=$(mktemp -d --tmpdir=$HERE)
mkdir $TMP_DIR/RELEASE
WORK=$TMP_DIR/RELEASE


if [[ $IS_BZR == 'true' ]]; then
    echo "Getting juju core and all its dependencies."
    GOPATH=$WORK go get -v -d $PACKAGE/... || \
        GOPATH=$WORK go get -v -d $PACKAGE/... || \
        GOPATH=$WORK go get -v -d $PACKAGE/...
    echo "Setting juju core tree to $JUJU_CORE_BRANCH $REVNO."
    (cd "$WORK/src/$PACKAGE/" &&
     bzr pull --no-aliases --remember --overwrite -r $REVNO $JUJU_CORE_BRANCH)
    # Devs moved a package.
    if [[ $JUJU_CORE_BRANCH == 'lp:juju-core/1.18' ]]; then
        echo "Moving deps to support 1.18 releases."
        GOPATH=$WORK go get -v github.com/errgo/errgo
        rm -rf $WORK/src/github.com/juju/errgo
    fi
else
    echo "Getting juju core from $JUJU_CORE_REPO."
    mkdir -p $WORK/src/$PACKAGE
    git clone $JUJU_CORE_REPO $WORK/src/$PACKAGE
    echo "Setting juju core tree to $REVISION."
    cd $WORK/src/$PACKAGE
    if git ls-remote ./  | grep origin/$REVISION; then
        git checkout origin/$REVISION
    else
        git checkout $REVISION
    fi
    if [[ "$MERGE_REF" != "" && "$MERGE_REPO" != "" ]]; then
        if [[ "$MERGE_REV" != "" ]]; then
            echo "Merging $MERGE_REV in $MERGE_REF from $MERGE_REPO into $REVISION"
            git fetch $MERGE_REPO $MERGE_REF
            git merge $MERGE_REV
        else
            echo "Pulling $MERGE_REF from $MERGE_REPO into $REVISION"
            git pull $MERGE_REPO $MERGE_REF
        fi
    fi
    echo "Getting juju core's dependencies."
    GOPATH=$WORK go get -v -d ./... || \
        GOPATH=$WORK go get -v -d ./... || \
        GOPATH=$WORK go get -v -d ./... 
    cd $HERE
fi


echo "Updating juju-core dependencies to the required versions."
GOPATH=$WORK go get -v launchpad.net/godeps
GODEPS=$WORK/bin/godeps
if [[ ! -f $GODEPS ]]; then
    echo "! Could not install godeps."
    exit 1
fi
GOPATH=$WORK $GODEPS -u "$WORK/src/$PACKAGE/dependencies.tsv"
# Remove godeps.
rm -r $WORK/bin

# Change the generic release to the proper juju-core version.
VERSION=$(sed -n 's/^const version = "\(.*\)"/\1/p' \
    $WORK/src/$PACKAGE/version/version.go)
mv $WORK $TMP_DIR/juju-core_${VERSION}/

# Tar it up.
TARFILE=$(pwd)/juju-core_${VERSION}.tar.gz
cd $TMP_DIR
tar cfz $TARFILE --exclude .hg --exclude .git --exclude .bzr juju-core_${VERSION}

echo "release tarball: $TARFILE"
