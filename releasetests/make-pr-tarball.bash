#!/bin/bash

# Assemble a source tree using git to checkout the revision,
# then go to get the package deps, then godeps to pin the versions/
# Lastly create a release tarball from the tree.
set -e

unset GOPATH
unset GOBIN

DEFAULT_GIT_JUJU_CORE="https://github.com/juju/juju.git"
PACKAGE="github.com/juju/juju"

SCRIPT_DIR=$(cd $(dirname "${BASH_SOURCE[0]}") && pwd )

usage() {
    echo "usage: $0 <GIT_PR> [GIT_REPO]"
    echo "  GIT_PR: The juju core PR number to build"
    echo "  GIT_REPO: The juju core git repo; defaults to $DEFAULT_GIT_JUJU_CORE"
    exit 1
}

GIT_PR=$1
JUJU_CORE_REPO=${2:-$DEFAULT_GIT_JUJU_CORE}

HERE=$(pwd)
TMP_DIR=$(mktemp -d --tmpdir=$HERE)
mkdir $TMP_DIR/RELEASE
WORK=$TMP_DIR/RELEASE
WORKPACKAGE=$WORK/src/$PACKAGE

echo "Getting juju core from $JUJU_CORE_REPO."
mkdir -p $WORKPACKAGE
git clone $JUJU_CORE_REPO $WORKPACKAGE
cd $WORKPACKAGE
echo "Checking out PR revision."
git fetch --tags $JUJU_CORE_REPO +refs/pull/$GIT_PR/merge:refs/remotes/origin-pull/pull/$GIT_PR/merge
MERGE_COMMIT=$(git rev-parse refs/remotes/origin-pull/pull/$GIT_PR/merge^{commit})
git checkout -f $MERGE_COMMIT

# Build juju in directory
source "$SCRIPT_DIR/build-juju-source.bash"

# Tar it up.
echo "Creating build tarball"
cd $TMP_DIR
TARFILE=$HERE/juju-core_${VERSION}.tar.gz
tar cfz $TARFILE --exclude .hg --exclude .git --exclude .bzr juju-core_${VERSION}

echo "Successfully created tarball: $TARFILE"
