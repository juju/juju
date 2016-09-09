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

echo "Getting and updating juju-core dependencies to the required versions."
GOPATH=$WORK go get -v github.com/rogpeppe/godeps
GODEPS=$WORK/bin/godeps
if [[ ! -f $GODEPS ]]; then
    echo "! Could not install godeps."
    exit 1
fi
GOPATH=$WORK $GODEPS -u "$WORKPACKAGE/dependencies.tsv"

# Remove godeps, and non-free data
echo "Removing godeps and non-free data."
rm -rf $WORK/src/github.com/rogpeppe/godeps
rm -rf $WORK/src/github.com/kisielk
rm -rf $WORK/src/code.google.com/p/go.net/html/charset/testdata/
rm -f $WORK/src/code.google.com/p/go.net/html/charset/*test.go
rm -rf $WORK/src/golang.org/x/net/html/charset/testdata/
rm -f $WORK/src/golang.org/x/net/html/charset/*test.go
rm -rf $WORK/src/github.com/prometheus/procfs/fixtures

# Remove backup files that confuse lintian.
echo "Removing backup files"
find $WORK/src/ -type f -name *.go.orig -delete

# Validate the go src tree against dependencies.tsv
echo "Validating dependencies.tsv"
$SCRIPT_DIR/check_dependencies.py --delete-unknown --ignore $PACKAGE \
    "$WORKPACKAGE/dependencies.tsv" "$WORK/src"

# Apply patches against the whole source tree from the juju project
echo "Applying Patches"
if [[ -d "$WORKPACKAGE/patches" ]]; then
    $SCRIPT_DIR/apply_patches.py "$WORKPACKAGE/patches" "$WORK/src"
fi

# Run juju's fmt and vet script on the source after finding the right version
echo "Running format and checking build"
if [[ $(lsb_release -sc) == "trusty" ]]; then
    CHECKSCRIPT=./scripts/verify.bash
    if [[ ! -f $WORKPACKAGE/scripts/verify.bash ]]; then
        CHECKSCRIPT=./scripts/pre-push.bash
    fi
    (cd $WORKPACKAGE && GOPATH=$WORK $CHECKSCRIPT)
fi

# Remove binaries and build artefacts
echo "Removing binaries and build artifacts"
rm -r $WORK/bin
if [[ -d $WORK/pkg ]]; then
    rm -r $WORK/pkg
fi

echo "Rename to proper release version"
VERSION=$(sed -n 's/^const version = "\(.*\)"/\1/p' \
    $WORKPACKAGE/version/version.go)

# Change the generic release to the proper juju-core version.
mv $WORK $TMP_DIR/juju-core_${VERSION}/

# Tar it up.
echo "Creating build tarball"
cd $TMP_DIR
TARFILE=$(pwd)/juju-core_${VERSION}.tar.gz
tar cfz $TARFILE --exclude .hg --exclude .git --exclude .bzr juju-core_${VERSION}

echo "Successfully created tarball: $TARFILE"
