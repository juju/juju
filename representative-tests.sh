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
    NETWORK="echo '1.x does not support networking tests'"
    GRANT="echo '1.x does not support grant revoke'"
    RACE="echo '1.x does not pass race unit tests'"
else
    mkdir -p $WORKSPACE/artifacts/network
    mkdir -p $WORKSPACE/artifacts/grant
    NETWORK="timeout -s INT 20m $SCRIPTS/assess_network_health.py parallel-rackspace $JUJU_BIN $WORKSPACE/artifacts/network merge-juju-network --series xenial --bundle 'cs:bundle/mediawiki-single'"
    GRANT="timeout -s INT 20m $SCRIPTS/assess_user_grant_revoke.py parallel-lxd $JUJU_BIN $WORKSPACE/artifacts/grant merge-juju-grant --timeout 1500 --series xenial"
    RACE="run-unit-tests c4.4xlarge $XENIAL_AMI --force-archive --race --local $TARFILE_NAME"
    RACE="echo 'Skipping race unit tests.'"
fi
timeout 180m concurrently.py -v -l $WORKSPACE/artifacts \
    xenial="$SCRIPTS/run-unit-tests c4.4xlarge $XENIAL_AMI --local $TARFILE_NAME --use-tmpfs --force-archive" \
    windows="$SCRIPTS/gotestwin.py developer-win-unit-tester.vapour.ws $TARFILE_NAME github.com/juju/juju/cmd" \
    network="$NETWORK" \
    grant="$GRANT" \
    race="$RACE"
