#!/bin/bash
set -eu
RELEASE_SCRIPTS=$HOME/juju-release-tools
SCRIPTS=$HOME/juju-ci-tools
WORKSPACE=$(pwd)
JUJU_HOME=$HOME/.juju
source $HOME/.bashrc
source $HOME/cloud-city/juju-qa.jujuci
set -x

$SCRIPTS/jujuci.py setup-workspace --clean-env testing-osx-client $WORKSPACE
~/Bin/juju destroy-environment --force -y testing-osx-client || true
TARFILE=$($SCRIPTS/jujuci.py get build-osx-client 'juju-*-osx.tar.gz' ./)
echo "Downloaded $TARFILE"
tar -xf ./$TARFILE -C $WORKSPACE

export PATH=$WORKSPACE/juju-bin:$PATH
$SCRIPTS/deploy_stack.py testing-osx-client
EXIT_STATUS=$?
juju destroy-environment -y testing-osx-client || true
exit $EXIT_STATUS
