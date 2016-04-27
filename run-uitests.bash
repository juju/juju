#!/bin/bash
set -eu
export USER=jenkins
export SCRIPTS=$HOME/juju-ci-tools
export RELEASE_TOOLS=$HOME/juju-release-tools
export JUJU_UITEST=$HOME/eco-repos/src/github.com/CanonicalLtd/juju-uitest
export CLOUD_CITY=$HOME/cloud-city
export S3_CONFIG=$CLOUD_CITY/juju-qa.s3cfg
source $CLOUD_CITY/staging-charm-store.rc
source $CLOUD_CITY/juju-qa.jujuci
set -x

export PATH=/usr/lib/go-1.6/bin:$PATH

revision_build=${1:-$revision_build}
JOB_NAME=${2:-juju-with-uitest}
WORKSPACE=${3:-$WORKSPACE}
if [[ -z $WORKSPACE || $WORKSPACE == './' || $WORKSPACE == './' ]]; then
    echo "Set a sane WORKSPACE: [$WORKSPACE] is not safe."
    exit 1
fi
WORKSPACE=$(readlink -f $WORKSPACE)

$SCRIPTS/jujuci.py -v setup-workspace $WORKSPACE
export TMPDIR="$WORKSPACE/tmp"
mkdir $TMPDIR

source $($SCRIPTS/s3ci.py get --config $S3_CONFIG $revision_build build-revision buildvars.bash)

# Get the pristine tarfile and unpack it to build a testing juju.
JUJU_ARCHIVE=$($SCRIPTS/s3ci.py get --config $S3_CONFIG $revision_build build-revision juju-core_.*.tar.gz)
tar -xf $JUJU_ARCHIVE
JUJU_DIR=$(basename $JUJU_ARCHIVE .tar.gz)
export GOPATH=$WORKSPACE/$JUJU_DIR
JUJU_PACKAGE=$GOPATH/src/github.com/juju/juju
CHARM_PACKAGE=$GOPATH/src/github.com/juju/charm
# Add the charm package to the tree.
#cp -r $HOME/eco-repos/src/github.com/juju/charm $CHARM_PACKAGE
go get github.com/juju/charm/...

# ^ Fails because uitest requires a git tree to make changes to.

# If tree must be tampered with, then let it be us who do it to ensure
# we understand what is not being tested.
CSCLIENT="$GOPATH/src/gopkg.in/juju/charmrepo.v2-unstable/csclient/csclient.go"
SERVER_URL="s,\"https://api.jujucharms.com/charmstore\",\"$STORE_URL\","
sed -i -e "$SERVER_URL" $CSCLIENT
for PACKAGE in $JUJU_PACKAGE $CHARM_PACKAGE; do
    cd $PACKAGE
    go install ./...
done
cd $WORKSPACE
ls $GOPATH/bin

# SHORT_REVISION=$(echo $REVISION_ID|head -c7)
# echo Building $BRANCH revision $SHORT_REVISION
# branch_url=$(echo $BRANCH | sed -r 's,gitbranch:[^:]*:(github.com/[^/]+/juju),https://\1,')
# $RELEASE_TOOLS/make-release-tarball.bash $SHORT_REVISION $branch_url
# GO_DIR=$(ls -d tmp.*/juju-core_$VERSION)
# export GOPATH=$WORKSPACE/$GO_DIR

# ^ Fails because sees uncommitted changes made by the purge of
#   undocumented deps and non-free files.

GUI_URL=$(sstream-query http://streams.canonical.com/juju/gui/streams/v1/index.json --output-format="%(item_url)s" | head -1)
GUI_ARCHIVE=$(basename $GUI_URL)
wget $GUI_URL

export JUJU_DATA=$CLOUD_CITY/jes-homes/$JOB_NAME
export JUJU_HOME=$JUJU_DATA
test -d $JUJU_DATA && rm -r $JUJU_DATA
mkdir -p $JUJU_DATA
cat << EOT > $JUJU_DATA/credentials.yaml
credentials:
  google:
    default-region: us-central1
    default-credential: juju-qa
    juju-qa:
      auth-type: jsonfile
      file: $CLOUD_CITY/gce-4f8322be6f89.json
EOT

cp -r $JUJU_UITEST $WORKSPACE/juju-uitest
cd $WORKSPACE/juju-uitest
make

SUITE="not TestStorefront"
SUITE="TestJujuCore"

# ^ We require TestJujuCore. TestCharm is fast.
#   TestStorefront fails, maybe because of phantom. firefox reports
#   Xvfb did not start, but xfvb works for the win client job.
#   SUITE cannot be an empty string if non-essential suites will fail.
# see http://pytest.org/latest/example/markers.html#using-k-expr-to-select-tests-based-on-their-name

# Do not reveal credentials.
echo "devenv/bin/uitest --driver phantom \
    -c google \
    --gui-archive $WORKSPACE/$GUI_ARCHIVE \
    --gopath $GOPATH \
    --credentials <SECRET> \
    --admin <SECRET> \
    --url $STORE_URL" \
    "$SUITE"
#    --juju-branch $REVISION_ID \
set +x
devenv/bin/uitest --driver phantom \
    -c google \
    --gui-archive $WORKSPACE/$GUI_ARCHIVE \
    --gopath $GOPATH \
    --credentials $STORE_CREDENTIALS \
    --admin $STORE_ADMIN \
    --url $STORE_URL \
    --failfast --debug \
    "$SUITE"

