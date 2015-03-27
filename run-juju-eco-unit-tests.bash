#!/bin/bash
set -eux

WORKSPACE=$(readlink -f $1)  # Where to build the tree.
ECO_BRANCH=$(readlink -f $2)  # Path to the repo: ~/Work/foo
ECO_PROJECT=$3  # Go Package name: github.com/juju/foo
BUILD_REVISION=$4 # The Juju CI build-revision number

: ${CI_TOOLS=$(readlink -f $(dirname $0))}
RELEASE_TOOLS=$(readlink -f $CI_TOOLS/../juju-release-tools)
CLOUD_CITY=$(readlink -f $CI_TOOLS/../cloud-city)

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
    esac
    shift
done

export PATH="$RELEASE_TOOLS:$CI_TOOLS:$PATH"


set +x
source $CLOUD_CITY/juju-qa.jujuci
set -x
jujuci.py get -b $BUILD_REVISION build-revision \
    buildvars.bash $WORKSPACE/
source $WORKSPACE/buildvars.bash
JUJU_REVISION=$REVISION_ID

GOPATH=$WORKSPACE/ecosystem
export GOPATH
ECO_PATH="$GOPATH/src/$(dirname $ECO_PROJECT)"
PACKAGE=$(basename $ECO_PROJECT)

# Assemble Juju.
mkdir -p $GOPATH/src/github.com/juju
cd $GOPATH/src/github.com/juju
git clone http://github.com/juju/juju.git juju
cd juju
git checkout $JUJU_REVISION

# Assemble the eco project.
mkdir -p $ECO_PATH
cd $ECO_PATH
git clone $ECO_BRANCH $PACKAGE
cd $PACKAGE
git checkout origin/master
SHORTHASH=$(git log --first-parent -1 --pretty=format:%h)

# Reassemble. We are still waiting for multifile godeps.
deptree.py -v \
    $GOPATH/src/github.com/juju/juju/dependencies.tsv \
    $ECO_PATH/$PACKAGE/dependencies.tsv

# Verify new juju doesn't break the eco project.
echo ""
echo "Testing $PACKAGE $SHORTHASH with Juju $JUJU_REVISION"
set +e
go test $ECO_PROJECT/...
EXITCODE=$?
if [[ $((EXITCODE)) == 0 ]]; then
    echo "SUCCESS"
else
    echo "FAIL"
fi
exit $EXITCODE
