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
    echo "usage: $0 <GIT_REV> [GIT_REPO] [MERGE_REF] [MERGE_REPO] [MERGE_REV]"
    echo "  GIT_REV: The juju core git revision or branch to build"
    echo "  GIT_REPO: The juju core git repo; defaults to $DEFAULT_GIT_JUJU_CORE"
    echo "  MERGE_REF: The git branch or tag to merge"
    echo "  MERGE_REPO: The git repo that contains the MERGE_REF"
    echo "  MERGE_REV: The optional git specific commit to merge"
    exit 1
}

MERGE_REPO=""
MERGE_REF=""
MERGE_REV=""

REVISION_OR_BRANCH=$1
JUJU_CORE_REPO=${2:-$DEFAULT_GIT_JUJU_CORE}
if [[ $# -ge 4 ]]; then
    MERGE_REF=$3
    MERGE_REPO=$4
    if [[ $# -eq 5 ]]; then
        MERGE_REV=$5
    fi
fi

HERE=$(pwd)
TMP_DIR=$(mktemp -d --tmpdir=$HERE)
mkdir $TMP_DIR/RELEASE
WORK=$TMP_DIR/RELEASE
WORKPACKAGE=$WORK/src/$PACKAGE

echo "Getting juju core from $JUJU_CORE_REPO."
mkdir -p $WORKPACKAGE
git clone $JUJU_CORE_REPO $WORKPACKAGE
cd $WORKPACKAGE
echo "Setting juju core tree to $REVISION_OR_BRANCH."
if git ls-remote ./  | grep origin/$REVISION_OR_BRANCH; then
    REVISION_OR_BRANCH=origin/$REVISION_OR_BRANCH
fi
git checkout $REVISION_OR_BRANCH
SHORT_REVISION=$(git rev-parse --short $REVISION_OR_BRANCH)
if [[ "$MERGE_REF" != "" && "$MERGE_REPO" != "" ]]; then
    if [[ "$MERGE_REV" != "" ]]; then
        echo "Merging $MERGE_REV in $MERGE_REF from $MERGE_REPO into $SHORT_REVISION"
        git fetch $MERGE_REPO $MERGE_REF
        git merge $MERGE_REV
    else
        echo "Pulling $MERGE_REF from $MERGE_REPO into $SHORT_REVISION"
        git pull $MERGE_REPO $MERGE_REF
    fi
fi
cd $HERE

# Build juju in directory
source "$SCRIPT_DIR/build-juju-source.bash"

# Tar it up.
echo "Creating build tarball"
TARFILE=$(pwd)/juju-core_${VERSION}.tar.gz
cd $TMP_DIR
tar cfz $TARFILE --exclude .hg --exclude .git --exclude .bzr juju-core_${VERSION}

echo "Successfully created tarball: $TARFILE"
