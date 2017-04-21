#!/bin/bash
set -eux
TARFILE=$(find $(pwd) -name juju-core*.tar.gz)
echo Using build tarball $TARFILE
TARFILE_NAME=$(basename "$TARFILE")

export GOPATH=$(dirname $(find $WORKSPACE -type d -name src -regex '.*juju-core[^/]*/src'))
if ! go install github.com/juju/juju/...; then
    exit 127
fi
export PATH=$(dirname $JUJU_BIN):$PATH
export GOCOOKIES=$WORKSPACE/.go-cookies
JUJU_BIN=$GOPATH/bin/juju

XENIAL_AMI=$($SCRIPTS/get_ami.py xenial amd64 --virt hvm)

VERSION=$($JUJU_BIN version | cut -d '-' -f1)
if [[ $VERSION =~ 1\..*  ]]; then
    LXD="echo '1.x does not support lxd'"
    GRANT="echo '1.x does not support grant revoke'"
    RACE="echo '1.x does not pass race unit tests'"
else
    mkdir -p $WORKSPACE/artifacts/lxd
    mkdir -p $WORKSPACE/artifacts/grant
    LXD="timeout -s INT 20m $SCRIPTS/deploy_job.py parallel-lxd $JUJU_BIN $WORKSPACE/artifacts/lxd merge-juju-lxd  --series xenial --debug"
    GRANT="echo \"Skipping Grant test for now.\""
    RACE="run-unit-tests c4.4xlarge $XENIAL_AMI --force-archive --race --local $TARFILE_NAME --install-deps 'golang-1.6 juju-mongodb distro-info-data ca-certificates bzr git-core mercurial zip golang-1.6-race-detector-runtime'"
    RACE="echo 'Skipping race unit tests.'"
fi
timeout 180m concurrently.py -v -l $WORKSPACE/artifacts \
    xenial="$SCRIPTS/run-unit-tests c4.4xlarge $XENIAL_AMI --local $TARFILE_NAME --use-tmpfs --force-archive" \
    windows="$SCRIPTS/gotestwin.py developer-win-unit-tester.vapour.ws $TARFILE_NAME github.com/juju/juju/cmd" \
    lxd="$LXD" \
    grant="$GRANT" \
    race="$RACE"
