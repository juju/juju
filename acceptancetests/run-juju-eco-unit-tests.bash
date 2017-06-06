#!/bin/bash
set -eux

WORKSPACE=$(readlink -f $1)  # Where to build the tree.
LOCAL_TREE=$(readlink -f $2)  # Path to the repo: ~/gogo/src
ECO_PROJECT=$3  # Go Package name: github.com/juju/foo

: ${CI_TOOLS=$(readlink -f $(dirname $0))}
RELEASE_TOOLS=$(readlink -f $CI_TOOLS/../juju-release-tools)
CLOUD_CITY=$(readlink -f $CI_TOOLS/../cloud-city)
JUJU_BUILD=""

while [[ "${1-}" != "" ]]; do
    case $1 in
        --release-tools)
            shift
            RELEASE_TOOLS=$1
            ;;
        --cloud-city)
            shift
            CLOUD_CITY=$1
            ;;
        --juju-build)
            shift
            JUJU_BUILD=$1
            ;;
    esac
    shift
done

GOPATH=$WORKSPACE/ecosystem
export GOPATH
export PATH="$RELEASE_TOOLS:$CI_TOOLS:$PATH"

JUJU_PROJECT="github.com/juju/juju"
JUJU_DEPS="$GOPATH/src/$JUJU_PROJECT/dependencies.tsv"
ECO_PATH="$GOPATH/src/$(dirname $ECO_PROJECT)"
ECO_PACKAGE=$(basename $ECO_PROJECT)
ECO_DEPS="$ECO_PATH/$ECO_PACKAGE/dependencies.tsv"

# Copy the local Go tree to the GOPATH and use master for the eco project.
mkdir -p $ECO_PATH
cp -rp $LOCAL_TREE $GOPATH
cd $ECO_PATH/$ECO_PACKAGE
git checkout master
SHORTHASH=$(git log --first-parent -1 --pretty=format:%h)

# Are we testing a new Juju revision with the eco project, or a new eco rev?
if [[ -n "$JUJU_BUILD" ]]; then
    set +x
    source $CLOUD_CITY/juju-qa.jujuci
    set -x
    source $(s3ci.py get $revision_build build-revision buildvars.bash)
    JUJU_REVISION=$REVISION_ID
else
    JUJU_REVISION=$(grep $JUJU_PROJECT $ECO_DEPS | cut -d $'\t' -f 3)
fi

# Update Juju to the version under test.
cd $GOPATH/src/$JUJU_PROJECT
git fetch
git checkout $JUJU_REVISION

# Reassemble. We are still waiting for multifile godeps.
# godeps exist with an error when it cannot fetch a newer revision.
deptree.py -v $JUJU_DEPS $ECO_DEPS || deptree.py -v $JUJU_DEPS $ECO_DEPS

# Verify Juju and the eco project are integrated.
echo ""
echo "Testing $ECO_PACKAGE $SHORTHASH with Juju $JUJU_REVISION"
set +e
go test $ECO_PROJECT/... -test.v -gocheck.vv ||\
  go test $ECO_PROJECT/... -test.v -gocheck.vv
EXITCODE=$?
if [[ $((EXITCODE)) == 0 ]]; then
    echo "SUCCESS"
else
    echo "FAIL"
fi
exit $EXITCODE
