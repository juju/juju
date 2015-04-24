#!/bin/bash
set -eux

WORKSPACE=$(readlink -f $1)  # Where to build the tree.
LOCAL_TREE=$(readlink -f $2)  # Path to the repo: ~gogo/src
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

export PATH="$RELEASE_TOOLS:$CI_TOOLS:$PATH"

GOPATH=$WORKSPACE/ecosystem
export GOPATH
ECO_PATH="$GOPATH/src/$(dirname $ECO_PROJECT)"
PACKAGE=$(basename $ECO_PROJECT)

# Copy the local Go tree to the GOPATH and use master for the eco project.
mkdir -p $ECO_PATH
cp -rp $LOCAL_TREE $GOPATH
cd $ECO_PATH/$PACKAGE
git checkout master
SHORTHASH=$(git log --first-parent -1 --pretty=format:%h)

# Are we testing a new Juju revision with the eco project, or a new eco rev?
if [[ -n "$JUJU_BUILD" ]]; then
    set +x
    source $CLOUD_CITY/juju-qa.jujuci
    set -x
    jujuci.py get -b $JUJU_BUILD build-revision \
        buildvars.bash $WORKSPACE/
    source $WORKSPACE/buildvars.bash
    # Set the base to juju when we are testing its revision.
    BASE="$GOPATH/src/github.com/juju/juju/dependencies.tsv"
    OVERLAY="$ECO_PATH/$PACKAGE/dependencies.tsv"
    JUJU_REVISION=$REVISION_ID
else
    # Set the base to eco when we are testing its revision.
    BASE="$ECO_PATH/$PACKAGE/dependencies.tsv"
    OVERLAY="$GOPATH/src/github.com/juju/juju/dependencies.tsv"
    JUJU_REVISION=$(grep github.com/juju/juju $BASE | cut -d $'\t' -f 3)
fi

# Update Juju to the version under test. The branch is set to master during
# pull to avoid cases where the current branch is not in the remote.
cd $GOPATH/src/github.com/juju/juju
git checkout master
git pull
git checkout $JUJU_REVISION

# Reassemble. We are still waiting for multifile godeps.
# godeps exist with an error when it cannot fetch a newer revision.
deptree.py -v $BASE $OVERLAY || deptree.py -v $BASE $OVERLAY


# Verify Juju and the eco project are integrated.
echo ""
echo "Testing $PACKAGE $SHORTHASH with Juju $JUJU_REVISION"
set +e
go test $ECO_PROJECT/... || go test $ECO_PROJECT/...
EXITCODE=$?
if [[ $((EXITCODE)) == 0 ]]; then
    echo "SUCCESS"
else
    echo "FAIL"
fi
exit $EXITCODE
