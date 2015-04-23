#!/bin/bash
set -eux

WORKSPACE=$(readlink -f $1)  # Where to build the tree.
ECO_BRANCH=$(readlink -f $2)  # Path to the repo: ~/Work/foo
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

# Assemble the eco project.
mkdir -p $ECO_PATH
cd $ECO_PATH
git clone $ECO_BRANCH $PACKAGE
cd $PACKAGE
git checkout origin/master
SHORTHASH=$(git log --first-parent -1 --pretty=format:%h)

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

# Assemble Juju.
mkdir -p $GOPATH/src/github.com/juju
cd $GOPATH/src/github.com/juju
git clone http://github.com/juju/juju.git juju
cd juju
git checkout $JUJU_REVISION

# Reassemble. We are still waiting for multifile godeps.
# godeps exist with an error when it cannot fetch a newer revision.
deptree.py -v $BASE $OVERLAY || deptree.py -v $BASE $OVERLAY


# Verify new juju doesn't break the eco project.
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
